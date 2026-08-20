package main

// The MCP bearer token (#183): where it comes from, and the case where it used
// to come from nowhere at all.

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spagu/ssg/internal/mcp"
)

// TestTheFlagSuppliesTheTokenWhenItIsGiven: an explicit argument is the most
// deliberate of the sources, so it is taken as-is and nothing is minted.
func TestTheFlagSuppliesTheTokenWhenItIsGiven(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	token, minted, err := resolveMCPToken("from-the-flag")
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if token != "from-the-flag" || minted {
		t.Fatalf("token = %q minted = %v, want the flag value unminted", token, minted)
	}
}

// TestTheEnvironmentSuppliesTheTokenWithoutAFlag: the deployment the docs
// recommend, and the one a secret belongs in — a command line is in `ps`, in
// the shell history and in the supervisor's own log.
func TestTheEnvironmentSuppliesTheTokenWithoutAFlag(t *testing.T) {
	t.Setenv(mcpTokenEnv, "from-the-environment")
	token, minted, err := resolveMCPToken("")
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if token != "from-the-environment" || minted {
		t.Fatalf("token = %q minted = %v, want the environment value unminted", token, minted)
	}
}

// TestTheFlagBeatsTheEnvironment: both set is not an error, it is a preference.
func TestTheFlagBeatsTheEnvironment(t *testing.T) {
	t.Setenv(mcpTokenEnv, "from-the-environment")
	token, _, err := resolveMCPToken("from-the-flag")
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if token != "from-the-flag" {
		t.Fatalf("token = %q, want the flag to win", token)
	}
}

// TestAnEmptyEnvironmentVariableStillMintsAToken is the reported trap. The unit
// file forgets the variable, the shell expands `--token="$SSG_MCP_TOKEN"` to an
// empty argument, and what used to happen next was: nothing. The endpoint came
// up unauthenticated behind a public reverse proxy and said "loopback only".
func TestAnEmptyEnvironmentVariableStillMintsAToken(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	token, minted, err := resolveMCPToken("")
	if err != nil {
		t.Fatalf("resolveMCPToken: %v", err)
	}
	if !minted || token == "" {
		t.Fatalf("token = %q minted = %v, want a freshly minted token", token, minted)
	}
	// Whitespace is not a secret either: a variable holding a stray newline is
	// the same accident with a different shape.
	t.Setenv(mcpTokenEnv, "   \n")
	if _, minted, err := resolveMCPToken("  "); err != nil || !minted {
		t.Fatalf("blank values must mint: minted = %v err = %v", minted, err)
	}
}

// TestEveryMintedTokenIsDifferent: a token reused across runs of the binary
// would be a shared default, which is the thing minting exists to avoid.
func TestEveryMintedTokenIsDifferent(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	first, _, err := resolveMCPToken("")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := resolveMCPToken("")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("two runs must not share a minted token")
	}
}

// testMCPServer is a server with no roles disabled and nothing wired — enough
// to mount the endpoint, which is all these tests exercise.
func testMCPServer() *mcp.Server {
	return mcp.NewServer(mcp.Options{Root: ".", Logf: func(string, ...any) {}})
}

// serveAndCollect starts the endpoint with flags and returns the announcement.
func serveAndCollect(t *testing.T, flags mcpNetFlags) (string, int) {
	t.Helper()
	var lines []string
	code := serveMCPEndpoint(testMCPServer(), flags, func(f string, a ...any) {
		lines = append(lines, sprintfLine(f, a...))
	})
	return strings.Join(lines, "\n"), code
}

// TestALoopbackEndpointIsAuthenticatedToo: the whole point of #183. A loopback
// listener is exactly the one a reverse proxy fronts, so it is the last place
// that should be allowed to run without a token.
func TestALoopbackEndpointIsAuthenticatedToo(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	out, code := serveAndCollect(t, mcpNetFlags{listen: "127.0.0.1:0"})
	if code != -1 {
		t.Fatalf("exit = %d, want the caller to carry on", code)
	}
	if !strings.Contains(out, "Authorization: Bearer ") {
		t.Fatalf("a loopback endpoint must announce a token:\n%s", out)
	}
	if !strings.Contains(out, mcpTokenEnv) {
		t.Errorf("a minted token must name the variable that would pin it:\n%s", out)
	}
	if strings.Contains(out, "Listening beyond localhost") {
		t.Errorf("loopback must not carry the exposure warning:\n%s", out)
	}
}

// TestASuppliedTokenIsNotAnnouncedAsMinted: the operator already knows it, and
// telling them to set the variable they just set is noise.
func TestASuppliedTokenIsNotAnnouncedAsMinted(t *testing.T) {
	t.Setenv(mcpTokenEnv, "supplied-by-the-environment")
	out, code := serveAndCollect(t, mcpNetFlags{listen: "0"}) // a bare port means localhost
	if code != -1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "Bearer supplied-by-the-environment") {
		t.Fatalf("the environment token must be the one served:\n%s", out)
	}
	if strings.Contains(out, "Minted for this run") {
		t.Errorf("a supplied token is not minted:\n%s", out)
	}
}

// TestTheEndpointRefusesARequestWithoutTheToken closes the loop: the token is
// announced *and* enforced, which is the property the report cared about.
func TestTheEndpointRefusesARequestWithoutTheToken(t *testing.T) {
	t.Setenv(mcpTokenEnv, "a-secret-nobody-sent")
	var addr string
	code := serveMCPEndpoint(testMCPServer(), mcpNetFlags{listen: "127.0.0.1:0"},
		func(f string, a ...any) {
			line := sprintfLine(f, a...)
			if i := strings.Index(line, "http://"); i >= 0 {
				rest := line[i+len("http://"):]
				if j := strings.Index(rest, "/mcp"); j >= 0 {
					addr = rest[:j]
				}
			}
		})
	if code != -1 || addr == "" {
		t.Fatalf("endpoint did not start: code = %d addr = %q", code, addr)
	}
	resp, err := http.Post("http://"+addr+"/mcp", "application/json",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for a request with no bearer token", resp.StatusCode)
	}
}

// TestNoStdioWithoutListenIsRefused: --no-stdio alone leaves no transport at
// all, which is a server nobody can reach rather than a quiet one.
func TestNoStdioWithoutListenIsRefused(t *testing.T) {
	if _, code := serveAndCollect(t, mcpNetFlags{noStdio: true}); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	if _, code := serveAndCollect(t, mcpNetFlags{}); code != -1 {
		t.Fatalf("no --listen at all must simply carry on, got %d", code)
	}
}

// TestAnUnbindableAddressIsReported: a port already held elsewhere must fail
// loudly rather than leave the operator believing the endpoint is up.
func TestAnUnbindableAddressIsReported(t *testing.T) {
	if _, code := serveAndCollect(t, mcpNetFlags{listen: "256.256.256.256:1"}); code != 1 {
		t.Fatalf("exit = %d, want 1 for an address that cannot be bound", code)
	}
}

// sprintfLine renders one logf call the way the real logger would, so a test
// can assert on the finished line rather than on the format string.
func sprintfLine(format string, a ...any) string {
	return fmt.Sprintf(format, a...)
}

// TestAnExposedEndpointStillCarriesTheWarning: minting everywhere must not blur
// the one line that says this address is reachable from off the machine.
func TestAnExposedEndpointStillCarriesTheWarning(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	out, code := serveAndCollect(t, mcpNetFlags{listen: ":0"}) // no host: every interface
	if code != -1 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out, "Listening beyond localhost") {
		t.Fatalf("an exposed bind must warn:\n%s", out)
	}
	if !strings.Contains(out, "Authorization: Bearer ") {
		t.Errorf("and still be authenticated:\n%s", out)
	}
	if !strings.Contains(out, "Browser origins are refused") {
		t.Errorf("with no --allow-origin the refusal must be stated:\n%s", out)
	}
}

// TestAnAllowedOriginSuppressesTheOriginNotice: having named one, the operator
// does not need telling that the others are refused.
func TestAnAllowedOriginSuppressesTheOriginNotice(t *testing.T) {
	t.Setenv(mcpTokenEnv, "tok")
	out, _ := serveAndCollect(t, mcpNetFlags{listen: "127.0.0.1:0", origins: []string{"https://example.com"}})
	if strings.Contains(out, "Browser origins are refused") {
		t.Errorf("origins were configured:\n%s", out)
	}
}

// TestAnUnmintableTokenStopsTheEndpoint: minting is now the last line of
// defence, so a machine that cannot mint must not fall back to serving openly.
func TestAnUnmintableTokenStopsTheEndpoint(t *testing.T) {
	t.Setenv(mcpTokenEnv, "")
	old := newMCPToken
	t.Cleanup(func() { newMCPToken = old })
	newMCPToken = func() (string, error) { return "", errors.New("no entropy") }

	if _, _, err := resolveMCPToken(""); err == nil {
		t.Fatal("a failed mint must be reported, not swallowed")
	}
	if _, code := serveAndCollect(t, mcpNetFlags{listen: "127.0.0.1:0"}); code != 1 {
		t.Fatalf("exit = %d, want 1 rather than an unauthenticated endpoint", code)
	}
}

// TestWorkingDirOrDotFallsBackWhenThereIsNoDirectory: the pasted config snippet
// needs *something*, and a deleted working directory must not panic the help.
func TestWorkingDirOrDotFallsBackWhenThereIsNoDirectory(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "gone")
	if err := os.Mkdir(inner, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(inner)
	if err := os.Remove(inner); err != nil {
		t.Skipf("cannot remove the working directory on this platform: %v", err)
	}
	if got := workingDirOrDot(); got != "." {
		t.Fatalf("workingDirOrDot() = %q, want the fallback", got)
	}
}

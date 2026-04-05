Name:           ssg
Version:        1.7.8
Release:        1%{?dist}
Summary:        Fast static site generator written in Go

License:        BSD-3-Clause
URL:            https://github.com/spagu/ssg
Source0:        https://github.com/spagu/ssg/archive/refs/tags/v%{version}.tar.gz

BuildRequires:  golang >= 1.26
BuildRequires:  git

%description
SSG is a fast static site generator written in Go.
It converts Markdown content with YAML frontmatter to static HTML.

Features:
- Built-in HTTP server with watch mode
- WebP image conversion
- Cloudflare Pages deployment support
- Multiple templates
- SEO-friendly URL generation

%prep
%autosetup -n ssg-%{version}

%build
go build -ldflags "-s -w -X main.Version=%{version}" -o ssg ./cmd/ssg

%install
install -Dm755 ssg %{buildroot}%{_bindir}/ssg
install -Dm644 README.md %{buildroot}%{_docdir}/%{name}/README.md
install -Dm644 CHANGELOG.md %{buildroot}%{_docdir}/%{name}/CHANGELOG.md
install -Dm644 man/ssg.1 %{buildroot}%{_mandir}/man1/ssg.1

%files
%license LICENSE
%doc README.md CHANGELOG.md
%{_bindir}/ssg
%{_mandir}/man1/ssg.1*

%changelog
* Mon Mar 30 2026 spagu <spagu@github.com> - 1.7.8-1
- Updated Go dependencies to latest versions
- Updated Alpine base image to 3.23
- Updated GitHub Actions to latest versions
- Added comprehensive man page (ssg.1)
- Updated snapcraft base to core24

* Wed Mar 05 2026 spagu <spagu@github.com> - 1.7.0-1
- Added optional gRPC connection for MDDB (--mddb-protocol=grpc)
- Added MDDB watch mode with checksum polling (--mddb-watch)
- Refactored MDDB client to interface pattern

* Wed Mar 05 2026 spagu <spagu@github.com> - 1.6.3-1
- Added configurable batch_size for MDDB pagination (--mddb-batch-size)
- Fixed GetByType to fetch all documents with pagination (was limited to 1000)

* Wed Mar 05 2026 spagu <spagu@github.com> - 1.6.1-1
- Fixed MDDB client to match actual API format (contentMd, meta, addedAt/updatedAt)
- Fixed install.sh download URL pattern

* Wed Mar 05 2026 spagu <spagu@github.com> - 1.6.0-1
- Added MDDB content source support (single and bulk fetch)
- CLI flags: --mddb-url, --mddb-collection, --mddb-key, --mddb-lang
- YAML config support for MDDB

* Fri Jan 17 2026 spagu <spagu@github.com> - 1.3.0-1
- Added built-in HTTP server
- Added watch mode for auto-rebuild
- Added WebP quality control parameter

* Thu Jan 16 2026 spagu <spagu@github.com> - 1.2.0-1
- Added GitHub Actions support
- Added custom directory paths
- Added FreeBSD support

* Mon Jan 13 2026 spagu <spagu@github.com> - 1.1.0-1
- Added WebP image conversion
- Added ZIP deployment package
- Added Cloudflare Pages support

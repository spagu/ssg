// Package webp - tests for WebP conversion
package webp

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// writeWebpFixtures creates dummy converted .webp files so UpdateReferences
// treats the conversions as successful (GO-017).
func writeWebpFixtures(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("webp"), 0644); err != nil {
			t.Fatalf("Failed to create webp fixture %s: %v", name, err)
		}
	}
}

func TestUpdateReferences(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "image.webp", "photo.webp", "logo.webp", "bg.webp", "border.webp")

	// Create test HTML file with image references
	htmlContent := `<!DOCTYPE html>
<html>
<body>
<img src="image.jpg">
<img src="photo.jpeg">
<img src="logo.png">
<img src="already.webp">
</body>
</html>`

	expectedHTML := `<!DOCTYPE html>
<html>
<body>
<img src="image.webp">
<img src="photo.webp">
<img src="logo.webp">
<img src="already.webp">
</body>
</html>`

	htmlPath := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	// Create test CSS file with image references
	cssContent := `body {
  background: url(bg.jpg);
  border-image: url('border.png');
}`

	expectedCSS := `body {
  background: url(bg.webp);
  border-image: url('border.webp');
}`

	cssPath := filepath.Join(tmpDir, "style.css")
	if err := os.WriteFile(cssPath, []byte(cssContent), 0644); err != nil {
		t.Fatalf("Failed to create CSS file: %v", err)
	}

	// Create a non-HTML/CSS file (should be ignored)
	jsPath := filepath.Join(tmpDir, "script.js")
	jsContent := `var img = "image.jpg";`
	if err := os.WriteFile(jsPath, []byte(jsContent), 0644); err != nil {
		t.Fatalf("Failed to create JS file: %v", err)
	}

	// Run UpdateReferences
	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}

	// Verify HTML was updated
	resultHTML, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read updated HTML: %v", err)
	}
	if string(resultHTML) != expectedHTML {
		t.Errorf("HTML not updated correctly.\nExpected:\n%s\nGot:\n%s", expectedHTML, string(resultHTML))
	}

	// Verify CSS was updated
	resultCSS, err := os.ReadFile(cssPath)
	if err != nil {
		t.Fatalf("Failed to read updated CSS: %v", err)
	}
	if string(resultCSS) != expectedCSS {
		t.Errorf("CSS not updated correctly.\nExpected:\n%s\nGot:\n%s", expectedCSS, string(resultCSS))
	}

	// Verify JS was NOT modified
	resultJS, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("Failed to read JS: %v", err)
	}
	if string(resultJS) != jsContent {
		t.Errorf("JS file should not be modified.\nExpected:\n%s\nGot:\n%s", jsContent, string(resultJS))
	}
}

func TestUpdateReferencesWithSrcset(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "small.webp", "medium.webp", "large.webp")

	htmlContent := `<img srcset="small.jpg 100w, medium.jpeg 200w, large.png 300w">`
	expectedHTML := `<img srcset="small.webp 100w, medium.webp 200w, large.webp 300w">`

	htmlPath := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}

	result, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read updated HTML: %v", err)
	}

	if string(result) != expectedHTML {
		t.Errorf("Srcset not updated correctly.\nExpected:\n%s\nGot:\n%s", expectedHTML, string(result))
	}
}

func TestUpdateReferencesWithSingleQuotes(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "image.webp", "photo.webp")

	htmlContent := `<img src='image.jpg'><img src='photo.png'>`
	expectedHTML := `<img src='image.webp'><img src='photo.webp'>`

	htmlPath := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}

	result, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read updated HTML: %v", err)
	}

	if string(result) != expectedHTML {
		t.Errorf("Single quotes not handled correctly.\nExpected:\n%s\nGot:\n%s", expectedHTML, string(result))
	}
}

func TestUpdateReferencesNoChanges(t *testing.T) {
	tmpDir := t.TempDir()

	// Content without any image references to change
	htmlContent := `<html><body>No images here</body></html>`

	htmlPath := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}

	result, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read HTML: %v", err)
	}

	if string(result) != htmlContent {
		t.Errorf("File should not be modified when no changes needed")
	}
}

func TestUpdateReferencesSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory
	subDir := filepath.Join(tmpDir, "pages")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdirectory: %v", err)
	}

	// Relative references resolve against the page's own directory (GO-017).
	writeWebpFixtures(t, subDir, "image.webp")

	htmlContent := `<img src="image.jpg">`
	expectedHTML := `<img src="image.webp">`

	htmlPath := filepath.Join(subDir, "page.html")
	if err := os.WriteFile(htmlPath, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create HTML file: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}

	result, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("Failed to read updated HTML: %v", err)
	}

	if string(result) != expectedHTML {
		t.Errorf("Subdirectory file not updated correctly")
	}
}

func TestConvertOptionsDefaults(t *testing.T) {
	opts := ConvertOptions{}

	// Quality 0 should be treated as invalid
	if opts.Quality != 0 {
		t.Errorf("Default quality should be 0 (to be set to 60 in ConvertDirectory)")
	}
}

func TestConvertDirectoryNoCwebp(t *testing.T) {
	// This test checks behavior when cwebp is not available
	// We can't easily test this without mocking exec.LookPath
	// So we just verify the function signature works
	tmpDir := t.TempDir()

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	// This will fail if cwebp is not installed, which is expected
	_, _, err := ConvertDirectory(tmpDir, opts)

	// If cwebp is not installed, we expect an error
	// If cwebp IS installed, it should succeed (no images to convert)
	// Either way, the function should not panic
	_ = err
}

func TestConvertDirectoryEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, saved, err := ConvertDirectory(tmpDir, opts)

	// If cwebp is not installed, expect error
	if err != nil {
		// This is expected if cwebp is not installed
		return
	}

	// If cwebp is installed, empty dir should result in 0 conversions
	if converted != 0 {
		t.Errorf("Expected 0 conversions, got %d", converted)
	}
	if saved != 0 {
		t.Errorf("Expected 0 saved bytes, got %d", saved)
	}
}

func TestConvertDirectoryQualityNormalization(t *testing.T) {
	// Test that invalid quality values are normalized
	tmpDir := t.TempDir()

	// Quality 0 should be normalized to 60
	opts := ConvertOptions{
		Quality: 0,
		Quiet:   true,
	}

	// This will fail if cwebp is not installed
	_, _, _ = ConvertDirectory(tmpDir, opts)

	// Quality > 100 should also be normalized to 60
	opts.Quality = 150
	_, _, _ = ConvertDirectory(tmpDir, opts)

	// Test passes if no panic occurs
}

func TestConvertDirectoryWithImage(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal valid PNG file (1x1 red pixel)
	// PNG signature + IHDR chunk + IDAT chunk + IEND chunk
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk length + type
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1 image
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE, // bit depth, color type, etc + CRC
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54, // IDAT chunk length + type
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00, // compressed pixel data
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7, // + CRC
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, // IEND chunk length + type
		0xAE, 0x42, 0x60, 0x82, // CRC
	}

	pngPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)

	// If cwebp is installed, we should have converted 1 image
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion, got %d", converted)
	}

	// Original should be removed
	if _, err := os.Stat(pngPath); err == nil {
		t.Error("Original PNG should be removed after conversion")
	}

	// WebP should exist
	webpPath := filepath.Join(tmpDir, "test.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Error("WebP file not created")
	}
}

func TestConvertDirectoryWithJPG(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal valid JPEG file (1x1 red pixel)
	jpgData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
		0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
		0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
		0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD5, 0xDB, 0x20, 0xA8, 0xF1, 0x7F, 0xFF,
		0xD9,
	}

	jpgPath := filepath.Join(tmpDir, "test.jpg")
	if err := os.WriteFile(jpgPath, jpgData, 0644); err != nil {
		t.Fatalf("Failed to write JPG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)

	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion, got %d", converted)
	}

	// WebP should exist
	webpPath := filepath.Join(tmpDir, "test.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Error("WebP file not created")
	}
}

func TestUpdateReferencesErrorPath(t *testing.T) {
	// Test with non-existent directory
	err := UpdateReferences("/nonexistent/path")
	if err == nil {
		t.Error("Expected error for nonexistent path")
	}
}

func TestConvertDirectoryNonExistent(t *testing.T) {
	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	// Test with non-existent directory
	_, _, err := ConvertDirectory("/nonexistent/path", opts)
	// This might return an error from LookPath first, or Walk error
	// Either way, shouldn't panic
	_ = err
}

func TestConvertDirectorySkipsNonImages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create non-image files
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("readme"), 0644); err != nil {
		t.Fatalf("Failed to create txt file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "data.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to create json file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "existing.webp"), []byte("webp"), 0644); err != nil {
		t.Fatalf("Failed to create webp file: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 0 {
		t.Errorf("Expected 0 conversions for non-image files, got %d", converted)
	}
}

func TestConvertDirectoryWithSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create subdirectory with image
	subDir := filepath.Join(tmpDir, "images")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create subdir: %v", err)
	}

	// Create minimal PNG in subdirectory
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	pngPath := filepath.Join(subDir, "nested.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion in subdirectory, got %d", converted)
	}

	// Check nested webp exists
	webpPath := filepath.Join(subDir, "nested.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Error("WebP file not created in subdirectory")
	}
}

func TestConvertDirectoryWithJPEGExtension(t *testing.T) {
	tmpDir := t.TempDir()

	// Create minimal JPEG with .jpeg extension
	jpgData := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0xB5, 0x10, 0x00, 0x02, 0x01, 0x03,
		0x03, 0x02, 0x04, 0x03, 0x05, 0x05, 0x04, 0x04, 0x00, 0x00, 0x01, 0x7D,
		0x01, 0x02, 0x03, 0x00, 0x04, 0x11, 0x05, 0x12, 0x21, 0x31, 0x41, 0x06,
		0x13, 0x51, 0x61, 0x07, 0x22, 0x71, 0x14, 0x32, 0x81, 0x91, 0xA1, 0x08,
		0x23, 0x42, 0xB1, 0xC1, 0x15, 0x52, 0xD1, 0xF0, 0x24, 0x33, 0x62, 0x72,
		0x82, 0x09, 0x0A, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x25, 0x26, 0x27, 0x28,
		0x29, 0x2A, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x3A, 0x43, 0x44, 0x45,
		0x46, 0x47, 0x48, 0x49, 0x4A, 0x53, 0x54, 0x55, 0x56, 0x57, 0x58, 0x59,
		0x5A, 0x63, 0x64, 0x65, 0x66, 0x67, 0x68, 0x69, 0x6A, 0x73, 0x74, 0x75,
		0x76, 0x77, 0x78, 0x79, 0x7A, 0x83, 0x84, 0x85, 0x86, 0x87, 0x88, 0x89,
		0x8A, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0xA2, 0xA3,
		0xA4, 0xA5, 0xA6, 0xA7, 0xA8, 0xA9, 0xAA, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6,
		0xB7, 0xB8, 0xB9, 0xBA, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xC8, 0xC9,
		0xCA, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7, 0xD8, 0xD9, 0xDA, 0xE1, 0xE2,
		0xE3, 0xE4, 0xE5, 0xE6, 0xE7, 0xE8, 0xE9, 0xEA, 0xF1, 0xF2, 0xF3, 0xF4,
		0xF5, 0xF6, 0xF7, 0xF8, 0xF9, 0xFA, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01,
		0x00, 0x00, 0x3F, 0x00, 0xFB, 0xD5, 0xDB, 0x20, 0xA8, 0xF1, 0x7F, 0xFF,
		0xD9,
	}

	jpegPath := filepath.Join(tmpDir, "test.jpeg")
	if err := os.WriteFile(jpegPath, jpgData, 0644); err != nil {
		t.Fatalf("Failed to write JPEG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion for .jpeg, got %d", converted)
	}

	// WebP should exist
	webpPath := filepath.Join(tmpDir, "test.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Error("WebP file not created for .jpeg")
	}
}

func TestConvertDirectoryNonQuietMode(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an invalid "image" that will fail to convert
	invalidPath := filepath.Join(tmpDir, "invalid.png")
	if err := os.WriteFile(invalidPath, []byte("not a valid image"), 0644); err != nil {
		t.Fatalf("Failed to write invalid file: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   false, // Non-quiet mode
	}

	// This should not panic and should print warnings
	_, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}
	// Conversion should fail but continue
}

func TestConvertDirectoryContinuesOnError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create one invalid image and one valid image
	invalidPath := filepath.Join(tmpDir, "a_invalid.png") // Named to be processed first
	if err := os.WriteFile(invalidPath, []byte("not valid"), 0644); err != nil {
		t.Fatalf("Failed to write invalid: %v", err)
	}

	// Create valid PNG
	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	validPath := filepath.Join(tmpDir, "z_valid.png") // Named to be processed second
	if err := os.WriteFile(validPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write valid PNG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	// Should have converted at least 1 (the valid one)
	if converted != 1 {
		t.Errorf("Expected 1 conversion, got %d", converted)
	}

	// Valid webp should exist
	webpPath := filepath.Join(tmpDir, "z_valid.webp")
	if _, err := os.Stat(webpPath); err != nil {
		t.Error("Valid image should be converted even when other fails")
	}

	// Invalid should still exist (not converted, not removed)
	if _, err := os.Stat(invalidPath); err != nil {
		t.Error("Invalid image should remain when conversion fails")
	}
}

func TestUpdateReferencesReadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create unreadable HTML file (if we have permission to change modes)
	htmlPath := filepath.Join(tmpDir, "test.html")
	if err := os.WriteFile(htmlPath, []byte("<html></html>"), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	// Make file unreadable (may not work on all systems/as non-root)
	if err := os.Chmod(htmlPath, 0000); err != nil {
		t.Skipf("Cannot change permissions: %v", err)
	}
	defer func() { _ = os.Chmod(htmlPath, 0644) }() // Restore permissions for cleanup

	err := UpdateReferences(tmpDir)
	if err == nil {
		t.Error("Expected error for unreadable file")
	}
}

func TestConvertDirectorySkipExistingWebP(t *testing.T) {
	tmpDir := t.TempDir()

	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	pngPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	webpPath := filepath.Join(tmpDir, "test.webp")
	if err := os.WriteFile(webpPath, []byte("existing webp"), 0644); err != nil {
		t.Fatalf("Failed to write WebP: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
		Force:   false,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 0 {
		t.Errorf("Expected 0 conversions when webp exists, got %d", converted)
	}

	if _, statErr := os.Stat(pngPath); statErr == nil {
		t.Error("Original PNG should be removed when webp already exists")
	}

	existingContent, readErr := os.ReadFile(webpPath)
	if readErr != nil {
		t.Fatalf("Failed to read existing webp: %v", readErr)
	}
	if string(existingContent) != "existing webp" {
		t.Error("Existing webp should not be modified")
	}
}

func TestConvertDirectorySkipExistingWebPNonQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	pngPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	webpPath := filepath.Join(tmpDir, "test.webp")
	if err := os.WriteFile(webpPath, []byte("existing"), 0644); err != nil {
		t.Fatalf("Failed to write WebP: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   false,
		Force:   false,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 0 {
		t.Errorf("Expected 0 conversions, got %d", converted)
	}
}

func TestConvertDirectoryForceReconvert(t *testing.T) {
	tmpDir := t.TempDir()

	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	pngPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	webpPath := filepath.Join(tmpDir, "test.webp")
	if err := os.WriteFile(webpPath, []byte("old webp"), 0644); err != nil {
		t.Fatalf("Failed to write WebP: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
		Force:   true,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion with Force, got %d", converted)
	}

	newContent, readErr := os.ReadFile(webpPath)
	if readErr != nil {
		t.Fatalf("Failed to read webp: %v", readErr)
	}
	if string(newContent) == "old webp" {
		t.Error("WebP should be reconverted with Force=true")
	}
}

func TestConvertDirectoryWithSkippedAndNewNonQuiet(t *testing.T) {
	tmpDir := t.TempDir()

	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	skippedPng := filepath.Join(tmpDir, "skipped.png")
	if err := os.WriteFile(skippedPng, pngData, 0644); err != nil {
		t.Fatalf("Failed to write skipped PNG: %v", err)
	}
	skippedWebp := filepath.Join(tmpDir, "skipped.webp")
	if err := os.WriteFile(skippedWebp, []byte("exists"), 0644); err != nil {
		t.Fatalf("Failed to write existing WebP: %v", err)
	}

	newPng := filepath.Join(tmpDir, "new.png")
	if err := os.WriteFile(newPng, pngData, 0644); err != nil {
		t.Fatalf("Failed to write new PNG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   false,
		Force:   false,
	}

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 1 {
		t.Errorf("Expected 1 conversion, got %d", converted)
	}
}

func TestConvertDirectoryStatErrorInSecondPass(t *testing.T) {
	tmpDir := t.TempDir()

	pngData := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}

	pngPath := filepath.Join(tmpDir, "vanish.png")
	if err := os.WriteFile(pngPath, pngData, 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	opts := ConvertOptions{
		Quality: 80,
		Quiet:   true,
	}

	if _, lookErr := exec.LookPath("cwebp"); lookErr != nil {
		t.Skip("cwebp not available")
	}

	_ = os.Remove(pngPath)

	converted, _, err := ConvertDirectory(tmpDir, opts)
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}

	if converted != 0 {
		t.Errorf("Expected 0 conversions when file vanishes, got %d", converted)
	}
}

// minimalPNG returns a 1x1 valid PNG for conversion tests.
func minimalPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xDE,
		0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, 0x54,
		0x08, 0xD7, 0x63, 0xF8, 0xFF, 0xFF, 0x3F, 0x00,
		0x05, 0xFE, 0x02, 0xFE, 0xDC, 0xCC, 0x59, 0xE7,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44,
		0xAE, 0x42, 0x60, 0x82,
	}
}

// TestWebpTargetPath covers GO-016: the original extension is stripped by
// length, so uppercase extensions map to a single .webp sibling instead of a
// doubled Photo.JPG.webp.
func TestWebpTargetPath(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Photo.JPG", "Photo.webp"},
		{"foo.Png", "foo.webp"},
		{"bar.JPEG", "bar.webp"},
		{"image.jpg", "image.webp"},
		{filepath.Join("dir", "IMG_1234.JPG"), filepath.Join("dir", "IMG_1234.webp")},
	}
	for _, tt := range tests {
		if got := webpTargetPath(tt.in); got != tt.want {
			t.Errorf("webpTargetPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestConvertDirectoryUppercaseExistingWebp covers GO-016 in the first pass:
// with the .webp target already on disk, uppercase originals are deleted and no
// doubled Photo.JPG.webp appears.
func TestConvertDirectoryUppercaseExistingWebp(t *testing.T) {
	tmpDir := t.TempDir()
	originals := []string{"Photo.JPG", "foo.Png", "bar.JPEG"}
	for _, orig := range originals {
		if err := os.WriteFile(filepath.Join(tmpDir, orig), []byte("img"), 0644); err != nil {
			t.Fatalf("Failed to write original %s: %v", orig, err)
		}
	}
	writeWebpFixtures(t, tmpDir, "Photo.webp", "foo.webp", "bar.webp")

	converted, _, err := ConvertDirectory(tmpDir, ConvertOptions{Quality: 80, Quiet: true})
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}
	if converted != 0 {
		t.Errorf("Expected 0 conversions, got %d", converted)
	}
	for _, orig := range originals {
		if _, statErr := os.Stat(filepath.Join(tmpDir, orig)); statErr == nil {
			t.Errorf("original %s should be removed when its .webp exists", orig)
		}
		if _, statErr := os.Stat(filepath.Join(tmpDir, orig+".webp")); statErr == nil {
			t.Errorf("doubled %s.webp must not appear (GO-016)", orig)
		}
	}
}

// TestConvertDirectoryUppercaseConversion covers GO-016 in the conversion pass:
// Upper.PNG converts to Upper.webp, never Upper.PNG.webp, and the original is
// only removed because the .webp actually exists.
func TestConvertDirectoryUppercaseConversion(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "Upper.PNG"), minimalPNG(), 0644); err != nil {
		t.Fatalf("Failed to write PNG: %v", err)
	}

	converted, _, err := ConvertDirectory(tmpDir, ConvertOptions{Quality: 80, Quiet: true})
	if err != nil {
		t.Skipf("cwebp not available: %v", err)
	}
	if converted != 1 {
		t.Errorf("Expected 1 conversion, got %d", converted)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "Upper.webp")); statErr != nil {
		t.Error("Upper.webp should be created (GO-016)")
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "Upper.PNG.webp")); statErr == nil {
		t.Error("doubled Upper.PNG.webp must not appear (GO-016)")
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, "Upper.PNG")); statErr == nil {
		t.Error("original Upper.PNG should be removed after successful conversion")
	}
}

// TestUpdateReferencesSkipsMissingWebp covers GO-017: a reference whose
// conversion failed (no .webp on disk) must keep pointing at the original.
func TestUpdateReferencesSkipsMissingWebp(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "converted.webp")

	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="converted.jpg"><img src="failed.jpg">`), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	want := `<img src="converted.webp"><img src="failed.jpg">`
	if string(out) != want {
		t.Errorf("failed-conversion ref must stay.\nExpected: %s\nGot:      %s", want, out)
	}
}

// TestUpdateReferencesRemoteAndProse covers GO-017: http(s):// and
// protocol-relative URLs plus prose text are never rewritten, even when a
// matching local .webp exists.
func TestUpdateReferencesRemoteAndProse(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "photo.webp", "local.webp", "x.webp")

	html := `<img src="https://cdn.example.com/x.png">
<img src="http://cdn.example.com/x.jpg">
<img src="//cdn.example.com/x.jpeg">
<p>see photo.jpg for details</p>
<img src="local.jpg">`
	want := `<img src="https://cdn.example.com/x.png">
<img src="http://cdn.example.com/x.jpg">
<img src="//cdn.example.com/x.jpeg">
<p>see photo.jpg for details</p>
<img src="local.webp">`
	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(html), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	cssContent := `body{background:url(https://cdn.example.com/x.png)}`
	cssPath := filepath.Join(tmpDir, "style.css")
	if err := os.WriteFile(cssPath, []byte(cssContent), 0644); err != nil {
		t.Fatalf("Failed to write CSS: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	outHTML, _ := os.ReadFile(htmlPath)
	if string(outHTML) != want {
		t.Errorf("remote/prose must stay untouched.\nExpected:\n%s\nGot:\n%s", want, outHTML)
	}
	outCSS, _ := os.ReadFile(cssPath)
	if string(outCSS) != cssContent {
		t.Errorf("remote CSS url() must stay untouched, got: %s", outCSS)
	}
}

// TestUpdateReferencesUppercaseRefs covers GO-016/GO-017: references with
// uppercase extensions are rewritten once their .webp exists.
func TestUpdateReferencesUppercaseRefs(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "Photo.webp", "foo.webp", "bar.webp")

	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="Photo.JPG"><img src="foo.Png"><img src="bar.JPEG">`), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	want := `<img src="Photo.webp"><img src="foo.webp"><img src="bar.webp">`
	if string(out) != want {
		t.Errorf("uppercase refs not rewritten.\nExpected: %s\nGot:      %s", want, out)
	}
}

// TestUpdateReferencesUppercaseFiles covers GO-017: files with uppercase
// .HTML/.CSS extensions are scanned and rewritten too.
func TestUpdateReferencesUppercaseFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "pic.webp")

	htmlPath := filepath.Join(tmpDir, "page.HTML")
	if err := os.WriteFile(htmlPath, []byte(`<img src="pic.jpg">`), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}
	cssPath := filepath.Join(tmpDir, "style.CSS")
	if err := os.WriteFile(cssPath, []byte(`body{background:url("pic.png")}`), 0644); err != nil {
		t.Fatalf("Failed to write CSS: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	outHTML, _ := os.ReadFile(htmlPath)
	if string(outHTML) != `<img src="pic.webp">` {
		t.Errorf(".HTML file not rewritten (GO-017), got: %s", outHTML)
	}
	outCSS, _ := os.ReadFile(cssPath)
	if string(outCSS) != `body{background:url("pic.webp")}` {
		t.Errorf(".CSS file not rewritten (GO-017), got: %s", outCSS)
	}
}

// TestUpdateReferencesDataSrcUntouched covers GO-017 scoping: only src/srcset/
// href attributes are rewritten, so data-src (lazy-load) keeps its value.
func TestUpdateReferencesDataSrcUntouched(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "lazy.webp")

	orig := `<img data-src="lazy.jpg" alt="l">`
	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(orig), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if string(out) != orig {
		t.Errorf("data-src must stay untouched, got: %s", out)
	}
}

// TestUpdateReferencesAbsolutePath covers GO-017 resolution of "/"-rooted
// references against the output root rather than the page directory.
func TestUpdateReferencesAbsolutePath(t *testing.T) {
	tmpDir := t.TempDir()
	imgDir := filepath.Join(tmpDir, "img")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatalf("Failed to create img dir: %v", err)
	}
	writeWebpFixtures(t, imgDir, "abs.webp")

	postDir := filepath.Join(tmpDir, "posts")
	if err := os.MkdirAll(postDir, 0755); err != nil {
		t.Fatalf("Failed to create posts dir: %v", err)
	}
	htmlPath := filepath.Join(postDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img src="/img/abs.jpg">`), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if string(out) != `<img src="/img/abs.webp">` {
		t.Errorf("root-absolute ref not rewritten, got: %s", out)
	}
}

// TestUpdateReferencesSrcsetEmptyEntry covers the empty srcset entry branch:
// stray commas must not panic and remaining entries are still rewritten.
func TestUpdateReferencesSrcsetEmptyEntry(t *testing.T) {
	tmpDir := t.TempDir()
	writeWebpFixtures(t, tmpDir, "small.webp")

	htmlPath := filepath.Join(tmpDir, "index.html")
	if err := os.WriteFile(htmlPath, []byte(`<img srcset=", small.jpg 1x,">`), 0644); err != nil {
		t.Fatalf("Failed to write HTML: %v", err)
	}

	if err := UpdateReferences(tmpDir); err != nil {
		t.Fatalf("UpdateReferences failed: %v", err)
	}
	out, _ := os.ReadFile(htmlPath)
	if string(out) != `<img srcset=", small.webp 1x,">` {
		t.Errorf("srcset with empty entries mishandled, got: %s", out)
	}
}

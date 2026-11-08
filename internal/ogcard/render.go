package ogcard

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Card is what one page puts on its card.
type Card struct {
	Eyebrow string // small line above the title: the category or section
	Title   string
	Footer  string // the site's name
}

// Sizes at the 630 px reference height; a taller or shorter card scales them.
const (
	refHeight    = 630.0
	padding      = 72.0
	accentBar    = 14.0
	smallSize    = 30.0
	titleMax     = 76.0
	titleMin     = 40.0
	titleStep    = 4.0
	lineSpacing  = 1.18
	logoMaxH     = 72.0
	sectionGap   = 28.0
	maxLogoRatio = 0.3 // of the card's width
)

// Render draws the card and encodes it as PNG or JPEG.
func Render(c Card, l Layout) ([]byte, error) {
	k := float64(l.Height) / refHeight
	px := func(v float64) int { return int(v * k) }
	canvas := image.NewRGBA(image.Rect(0, 0, l.Width, l.Height))
	paintBackground(canvas, l)
	draw.Draw(canvas, image.Rect(0, 0, px(accentBar), l.Height), image.NewUniform(l.Accent), image.Point{}, draw.Src)

	pad := px(padding)
	left, right := pad+px(accentBar), l.Width-pad
	top, bottom := pad, l.Height-pad

	small := newFace(l.Font, smallSize*k)
	// Footer row: the site's name left, the logo right.
	footerTop := bottom - lineHeight(small)
	textRight := right
	if l.Logo != nil {
		logo := imaging.Fit(l.Logo, int(float64(l.Width)*maxLogoRatio), px(logoMaxH), imaging.Lanczos)
		b := logo.Bounds()
		at := image.Pt(right-b.Dx(), bottom-b.Dy())
		draw.Draw(canvas, b.Add(at), logo, b.Min, draw.Over)
		footerTop = min(footerTop, at.Y)
		textRight = at.X - px(sectionGap)
	}
	if c.Footer != "" {
		drawLine(canvas, small, l.Text, left, bottom-lineHeight(small), fitLine(small, c.Footer, textRight-left))
	}
	if c.Eyebrow != "" {
		drawLine(canvas, small, l.Accent, left, top, fitLine(small, c.Eyebrow, right-left))
		top += lineHeight(small) + px(sectionGap)
	}
	drawTitle(canvas, l, c.Title, image.Rect(left, top, right, footerTop-px(sectionGap)), k)
	return encode(canvas, l)
}

// paintBackground fills the card: the flat colour, or the image cropped to
// fill with the colour laid over it at the overlay's strength.
func paintBackground(dst *image.RGBA, l Layout) {
	draw.Draw(dst, dst.Bounds(), image.NewUniform(l.Background), image.Point{}, draw.Src)
	if l.BackgroundImage == nil {
		return
	}
	photo := imaging.Fill(l.BackgroundImage, l.Width, l.Height, imaging.Center, imaging.Lanczos)
	draw.Draw(dst, dst.Bounds(), photo, image.Point{}, draw.Src)
	shade := l.Background
	shade.A = uint8(l.Overlay * 255) // #nosec G115 -- overlay is clamped to 0..1
	draw.Draw(dst, dst.Bounds(), image.NewUniform(premultiply(shade)), image.Point{}, draw.Over)
}

// drawTitle sets the title as large as the box allows: it steps the size down
// until the wrapped lines fit, and at the smallest size cuts the last line
// with an ellipsis. The block is centred vertically in the box.
func drawTitle(dst *image.RGBA, l Layout, title string, box image.Rectangle, k float64) {
	var face font.Face
	var lines []string
	for size := titleMax; size >= titleMin; size -= titleStep {
		face = newFace(l.Font, size*k)
		lines = wrap(face, title, box.Dx())
		if len(lines)*lineHeight(face) <= box.Dy() {
			break
		}
	}
	lines = clampLines(face, lines, max(1, box.Dy()/lineHeight(face)), box.Dx())
	y := box.Min.Y + (box.Dy()-len(lines)*lineHeight(face))/2
	for _, line := range lines {
		drawLine(dst, face, l.Text, box.Min.X, y, line)
		y += lineHeight(face)
	}
}

// newFace sizes the font. opentype.NewFace cannot fail — it only stores the
// options — so its always-nil error is not passed on.
func newFace(f *opentype.Font, size float64) font.Face {
	face, _ := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	return face
}

// lineHeight is the distance between baselines.
func lineHeight(f font.Face) int {
	return int(float64(f.Metrics().Height.Ceil()) * lineSpacing)
}

// drawLine draws text with its top edge at y.
func drawLine(dst *image.RGBA, f font.Face, c color.RGBA, x, y int, text string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f,
		Dot: fixed.P(x, y+f.Metrics().Ascent.Ceil())}
	d.DrawString(text)
}

func premultiply(c color.RGBA) color.RGBA {
	a := uint32(c.A)
	return color.RGBA{R: uint8(uint32(c.R) * a / 255), G: uint8(uint32(c.G) * a / 255), // #nosec G115 -- ≤ 255
		B: uint8(uint32(c.B) * a / 255), A: c.A} // #nosec G115 -- ≤ 255
}

func encode(img image.Image, l Layout) ([]byte, error) {
	var buf bytes.Buffer
	var err error
	if l.JPEG {
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: l.Quality})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img)
	}
	return buf.Bytes(), err
}

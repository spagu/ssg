package images

// Template-facing facade: dict/slice in, typed result out. These are the only
// entry points the generator registers, so templates never touch raw internals.

// ResizeDict implements the imageResize template helper.
func (p *Processor) ResizeDict(source string, opts map[string]any) (ImageResult, error) {
	r, err := ParseResize(opts)
	if err != nil {
		return ImageResult{}, err
	}
	return p.Resize(source, r)
}

// CropDict implements the imageCrop template helper.
func (p *Processor) CropDict(source string, opts map[string]any) (ImageResult, error) {
	r, err := ParseCrop(opts)
	if err != nil {
		return ImageResult{}, err
	}
	return p.Crop(source, r)
}

// FilterDict implements the imageFilter template helper.
func (p *Processor) FilterDict(source string, filters []any, opts map[string]any) (ImageResult, error) {
	fs, err := ParseFilters(filters)
	if err != nil {
		return ImageResult{}, err
	}
	enc, err := ParseEncode("imageFilter", opts)
	if err != nil {
		return ImageResult{}, err
	}
	return p.Filter(source, fs, enc)
}

// ProcessList implements the imageProcess template helper.
func (p *Processor) ProcessList(source string, ops []any) (ImageResult, error) {
	parsed, err := ParseOps(ops)
	if err != nil {
		return ImageResult{}, err
	}
	return p.Process(source, parsed)
}

// SrcSetDict implements the imageSrcSet template helper.
func (p *Processor) SrcSetDict(source string, opts map[string]any) (ImageSet, error) {
	s, err := ParseSrcSet(opts)
	if err != nil {
		return ImageSet{}, err
	}
	return p.SrcSet(source, s)
}

// PictureDict implements the imagePicture template helper.
func (p *Processor) PictureDict(source string, opts map[string]any) (ImagePicture, error) {
	s, err := ParsePicture(opts)
	if err != nil {
		return ImagePicture{}, err
	}
	return p.Picture(source, s)
}

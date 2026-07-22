package images

import (
	"fmt"
)

// The template adapter: parses `dict`/`slice` values from templates into typed
// requests, rejecting unknown keys so typos like "widht" fail loudly instead of
// being silently ignored.

// ParseResize parses the imageResize options dict.
func ParseResize(opts map[string]any) (request, error) {
	const helper = "imageResize"
	var r request
	for key, v := range opts {
		if ok, err := r.parseCommonOption(helper, key, v); err != nil {
			return r, err
		} else if ok {
			continue
		}
		var err error
		switch key {
		case "width":
			r.Width, err = optInt(helper, key, v)
		case "height":
			r.Height, err = optInt(helper, key, v)
		case "mode":
			r.Mode, err = optString(helper, key, v)
		default:
			return r, fmt.Errorf("%s: unknown option %q", helper, key)
		}
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// ParseCrop parses the imageCrop options dict (rect XOR anchor/focal).
func ParseCrop(opts map[string]any) (request, error) {
	const helper = "imageCrop"
	var r request
	for key, v := range opts {
		if ok, err := r.parseCommonOption(helper, key, v); err != nil {
			return r, err
		} else if ok {
			continue
		}
		var err error
		switch key {
		case "width":
			r.Width, err = optInt(helper, key, v)
		case "height":
			r.Height, err = optInt(helper, key, v)
		case "x":
			r.X, err = optInt(helper, key, v)
			r.HasRect = true
		case "y":
			r.Y, err = optInt(helper, key, v)
			r.HasRect = true
		default:
			return r, fmt.Errorf("%s: unknown option %q", helper, key)
		}
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// ParseFilters parses the imageFilter chain: a slice of dicts with name+amount.
func ParseFilters(list []any) ([]request, error) {
	const helper = "imageFilter"
	out := make([]request, 0, len(list))
	for i, item := range list {
		dict, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: filter %d must be a dict, got %T", helper, i, item)
		}
		var r request
		for key, v := range dict {
			var err error
			switch key {
			case "name":
				r.Name, err = optString(helper, key, v)
			case "amount":
				r.Amount, err = optFloat(helper, key, v)
			default:
				return nil, fmt.Errorf("%s: filter %d: unknown option %q", helper, i, key)
			}
			if err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// ParseEncode parses the trailing encode-options dict shared by imageFilter.
func ParseEncode(helper string, opts map[string]any) (request, error) {
	var r request
	for key, v := range opts {
		ok, err := r.parseCommonOption(helper, key, v)
		if err != nil {
			return r, err
		}
		if !ok {
			return r, fmt.Errorf("%s: unknown option %q", helper, key)
		}
	}
	return r, nil
}

// ParseOps parses the imageProcess pipeline: each element is a dict with "op".
func ParseOps(list []any) ([]request, error) {
	const helper = "imageProcess"
	out := make([]request, 0, len(list))
	for i, item := range list {
		dict, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: operation %d must be a dict, got %T", helper, i, item)
		}
		op, _ := dict["op"].(string)
		var r request
		var err error
		switch op {
		case "resize":
			r, err = ParseResize(withoutOp(dict))
		case "crop":
			r, err = ParseCrop(withoutOp(dict))
		case "filter":
			var fs []request
			fs, err = ParseFilters([]any{withoutOp(dict)})
			if err == nil {
				r = fs[0]
			}
		case "encode":
			r, err = ParseEncode(helper, withoutOp(dict))
		default:
			return nil, fmt.Errorf("%s: operation %d: op must be resize, crop, filter or encode (got %q)", helper, i, op)
		}
		if err != nil {
			return nil, fmt.Errorf("operation %d: %w", i, err)
		}
		r.Op = op
		out = append(out, r)
	}
	return out, nil
}

// withoutOp copies a dict minus the routing "op" key.
func withoutOp(dict map[string]any) map[string]any {
	out := make(map[string]any, len(dict))
	for k, v := range dict {
		if k != "op" {
			out[k] = v
		}
	}
	return out
}

// ParseSrcSet parses the imageSrcSet options dict.
func ParseSrcSet(opts map[string]any) (srcSetOptions, error) {
	const helper = "imageSrcSet"
	var s srcSetOptions
	for key, v := range opts {
		if ok, err := s.Base.parseCommonOption(helper, key, v); err != nil {
			return s, err
		} else if ok {
			continue
		}
		var err error
		switch key {
		case "widths":
			s.Widths, err = optIntList(helper, key, v)
		case "defaultWidth":
			s.DefaultWidth, err = optInt(helper, key, v)
		case "mode":
			s.Base.Mode, err = optString(helper, key, v)
		default:
			return s, fmt.Errorf("%s: unknown option %q", helper, key)
		}
		if err != nil {
			return s, err
		}
	}
	if err := s.Base.validateCommon(helper); err != nil {
		return s, err
	}
	return s, nil
}

// ParsePicture parses the imagePicture options dict (issue #43).
func ParsePicture(opts map[string]any) (pictureOptions, error) {
	const helper = "imagePicture"
	var s pictureOptions
	for key, v := range opts {
		if ok, err := s.Base.parseCommonOption(helper, key, v); err != nil {
			return s, err
		} else if ok {
			continue
		}
		var err error
		switch key {
		case "formats":
			s.Formats, err = optStringList(helper, key, v)
		case "widths":
			s.Widths, err = optIntList(helper, key, v)
		case "defaultWidth":
			s.DefaultWidth, err = optInt(helper, key, v)
		case "sizes":
			s.Sizes, err = optString(helper, key, v)
		case "alt":
			s.Alt, err = optString(helper, key, v)
		case "mode":
			s.Base.Mode, err = optString(helper, key, v)
		default:
			return s, fmt.Errorf("%s: unknown option %q", helper, key)
		}
		if err != nil {
			return s, err
		}
	}
	// Format is set per-source-format inside Picture, so the shared Base.Format
	// must stay empty; reject a stray top-level "format" to avoid confusion.
	if s.Base.Format != "" {
		return s, fmt.Errorf("%s: use \"formats\" (a list), not \"format\"", helper)
	}
	if err := s.Base.validateCommon(helper); err != nil {
		return s, err
	}
	return s, nil
}

// optStringList reads a list of strings (template slice yields []any).
func optStringList(helper, key string, v any) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		if strs, ok2 := v.([]string); ok2 {
			return strs, nil
		}
		return nil, fmt.Errorf("%s: option %q must be a list of strings, got %T", helper, key, v)
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		str, err := optString(helper, fmt.Sprintf("%s[%d]", key, i), item)
		if err != nil {
			return nil, err
		}
		out = append(out, str)
	}
	return out, nil
}

// optIntList reads a list of numbers (template slice yields []any).
func optIntList(helper, key string, v any) ([]int, error) {
	list, ok := v.([]any)
	if !ok {
		if ints, ok2 := v.([]int); ok2 {
			return ints, nil
		}
		return nil, fmt.Errorf("%s: option %q must be a list of numbers, got %T", helper, key, v)
	}
	out := make([]int, 0, len(list))
	for i, item := range list {
		n, err := optInt(helper, fmt.Sprintf("%s[%d]", key, i), item)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

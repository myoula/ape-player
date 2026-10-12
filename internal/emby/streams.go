package emby

// Use metadata from the selected version only; indices from different versions
// may refer to entirely different languages or subtitle formats.
func itemStreams(item Item, mediaID string) []Stream {
	for _, source := range item.MediaSources {
		if source.ID == mediaID {
			if len(source.MediaStreams) > 0 {
				return source.MediaStreams
			}
			if len(item.MediaSources) == 1 {
				return item.MediaStreams
			}
			return nil
		}
	}
	if len(item.MediaSources) == 0 {
		return item.MediaStreams
	}
	return nil
}

func mergeStreams(primary, fallback []Stream) []Stream {
	result := append([]Stream(nil), primary...)
	for _, s := range fallback {
		found := false
		for i := range result {
			t := &result[i]
			if t.Index != s.Index || t.Type != s.Type {
				continue
			}
			found = true
			if t.Title == "" {
				t.Title = s.Title
			}
			if t.DisplayTitle == "" {
				t.DisplayTitle = s.DisplayTitle
			}
			if t.Language == "" {
				t.Language = s.Language
			}
			if t.Codec == "" {
				t.Codec = s.Codec
			}
			if t.DeliveryUrl == "" {
				t.DeliveryUrl = s.DeliveryUrl
			}
			if t.DeliveryMethod == "" {
				t.DeliveryMethod = s.DeliveryMethod
			}
			if t.Channels == 0 {
				t.Channels = s.Channels
			}
			t.IsExternal = t.IsExternal || s.IsExternal
			t.IsDefault = t.IsDefault || s.IsDefault
			t.IsForced = t.IsForced || s.IsForced
			break
		}
		if !found {
			result = append(result, s)
		}
	}
	return result
}

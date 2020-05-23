package indexes

func OpenContentToplevel(log kitlog.Logger, r repo.Interface) (processing.MessageExtractor, error) {
	f := func(content map[string]interface{}) ([]string, error) {
		var outs []string

		for k, v := range content {
			if str, ok := v.(string); ok {
				outs = append(outs, string(enc.Encode(nil, []byte(k), []byte(str))))
			}
		}

		return outs, nil
	}

}

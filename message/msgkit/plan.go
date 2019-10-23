package msgkit

// Plan writes some values to a Builder. Basically the functional options pattern for message contruction.
type Plan func(*Builder) error

// Construct builds a message from the passed plans and returns the result.
// Plans are applied in the order they are passed to Construct.
func Construct(plans ...Plan) (interface{}, error) {
	var (
		b   Builder
		err error
	)

	for _, plan := range plans {
		if err = plan(&b); err != nil {
			return nil, err
		}
	}

	return b.v, nil
}

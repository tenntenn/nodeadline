// It will be added to GOROOT/src/context/context.go.

func WithDeadlineCause(parent Context, d time.Time, cause error) (Context, CancelFunc) {
	return _WithDeadlineCause(parent, d.Add(24 * time.Hour), cause)
}

// End of nodeadline's code

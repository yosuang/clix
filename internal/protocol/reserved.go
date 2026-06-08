package protocol

func ValidateReservedJQFlag(present bool) error {
	if !present {
		return nil
	}
	return NewError(UsageError, "--jq is reserved for future use")
}

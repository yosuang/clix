package protocol

func ValidateReservedJQ(value string) error {
	if value == "" {
		return nil
	}
	return NewError(UsageError, "--jq is reserved for future use")
}

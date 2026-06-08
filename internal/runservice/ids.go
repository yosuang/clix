package runservice

import "github.com/google/uuid"

type IDGenerator struct{}

func NewIDGenerator() IDGenerator {
	return IDGenerator{}
}

func (IDGenerator) NewRunID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	return "run_" + id.String(), nil
}

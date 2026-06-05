package protocol

import (
	"encoding/json"
	"fmt"
	"io"
)

func WriteJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func WriteJSONError(errOut io.Writer, err error) error {
	perr := AsError(err)
	return WriteJSON(errOut, map[string]any{
		"ok":      false,
		"code":    perr.Code,
		"message": perr.Message,
	})
}

func WriteTextError(errOut io.Writer, err error) error {
	_, writeErr := fmt.Fprintln(errOut, AsError(err).Error())
	return writeErr
}

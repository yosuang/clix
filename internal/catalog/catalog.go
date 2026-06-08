package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"github.com/yosuang/clix/internal/domain"
	"github.com/yosuang/clix/internal/protocol"
)

type AdapterValidator interface {
	ValidateAdapter(adapter string, config map[string]any) error
}

type Options struct {
	ToolsDir         string
	AdapterValidator AdapterValidator
}

type Catalog struct {
	Tools  []domain.Tool
	ByName map[string]domain.Tool
}

func (c Catalog) Get(name string) (domain.Tool, bool) {
	tool, ok := c.ByName[name]
	return tool, ok
}

func (c Catalog) ValidateInput(toolName string, input json.RawMessage) error {
	tool, ok := c.Get(toolName)
	if !ok {
		return protocol.NewError(protocol.ToolNotFound, "tool not found")
	}

	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return protocol.NewError(protocol.ValidationError, "input must be valid JSON")
	}
	if err := tool.ValidateInput(value); err != nil {
		return validationError(err)
	}
	return nil
}

type Loader struct {
	Options Options
}

func NewLoader(options Options) Loader {
	return Loader{Options: options}
}

func (l Loader) Load() (Catalog, error) {
	return Load(l.Options)
}

func emptyCatalog() Catalog {
	return Catalog{Tools: []domain.Tool{}, ByName: map[string]domain.Tool{}}
}

type acceptAdapterValidator struct{}

func (acceptAdapterValidator) ValidateAdapter(string, map[string]any) error {
	return nil
}

func validationError(err error) error {
	if requiredErr, required := requiredValidationError(err); required != nil && len(required.Missing) > 0 {
		path := append([]string{"input"}, requiredErr.InstanceLocation...)
		path = append(path, required.Missing[0])
		return protocol.NewError(protocol.ValidationError, fmt.Sprintf("%s is required", strings.Join(path, ".")))
	}
	return protocol.NewError(protocol.ValidationError, "input is invalid")
}

func requiredValidationError(err error) (*jsonschema.ValidationError, *kind.Required) {
	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return nil, nil
	}
	return findRequiredValidationError(validationErr)
}

func findRequiredValidationError(err *jsonschema.ValidationError) (*jsonschema.ValidationError, *kind.Required) {
	if required, ok := err.ErrorKind.(*kind.Required); ok {
		return err, required
	}
	for _, cause := range err.Causes {
		if matchErr, required := findRequiredValidationError(cause); required != nil {
			return matchErr, required
		}
	}
	return nil, nil
}

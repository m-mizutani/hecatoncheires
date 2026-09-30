package definition_test

import (
	"testing"

	"github.com/m-mizutani/goerr/v2"
	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/semantic/definition"
)

func TestErrInvalidValueSurvivesWrapping(t *testing.T) {
	err := goerr.Wrap(definition.ErrInvalidValue, "bad value", goerr.V("value", "x"))
	wrapped := goerr.Wrap(err, "field validation failed")
	gt.Error(t, wrapped).Is(definition.ErrInvalidValue)
}

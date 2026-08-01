package import_

import (
	"time"

	"boxpanel/internal/models"
)

func newID(prefix string) string { return models.NewID(prefix) }
func now() time.Time             { return models.Now() }

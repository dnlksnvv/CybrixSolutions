package validation

import (
	"fmt"
	"strings"

	derr "github.com/cybrix-solutions/agents-service/internal/domain/errors"
)

const templateAgentsFolderName = "Template Agents"

// ValidateFolderName проверяет имя обычной папки.
// Важно: виртуальная папка \"Template Agents\" не может быть создана/переименована через API.
func ValidateFolderName(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return derr.NewValidation("name", "required", "name is required", nil)
	}
	if len(n) > MaxLenName255 {
		limit := MaxLenName255
		return derr.NewValidation("name", "max_length_exceeded", fmt.Sprintf("name must be no longer than %d characters", limit), &limit)
	}
	if n == templateAgentsFolderName {
		return derr.NewBusiness("template_folder_is_virtual", "Template Agents is a virtual folder and cannot be created or modified")
	}
	return nil
}


package ids

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// New генерирует короткий стабильный ID с заданным префиксом.
// Мы используем криптографический random, чтобы избежать коллизий без обращения к базе.
// Формат: {prefix}{hex}, например: agent_9bb2ac714ff6733eabdc922bdc
func New(prefix string, bytesLen int) (string, error) {
	if bytesLen <= 0 {
		bytesLen = 12
	}
	b := make([]byte, bytesLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("id rand: %w", err)
	}
	return prefix + hex.EncodeToString(b), nil
}


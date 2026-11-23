package gotenbergtest

import "testing"

// Настоящий Gotenberg. Образа нет — Start пропускает тест. Образ не скачивается.
func TestS8_StartSkipsWithoutImage(t *testing.T) {
	_ = Start(t)
}

package e2e

import "testing"

// ВОПРОС: адрес core загрузчик берёт из окружения, а в deploy/secrets.md есть только CONTENT_API_TOKEN. Имени адреса нет. Варианты: не задавать адрес; выдумать переменную. Не выбираю. Вызов загрузчика по сети не тестирую.

func TestLD_R11_S6_LoaderCallsCore(t *testing.T) {
	t.Skip("ВОПРОС: в deploy/secrets.md нет имени переменной с адресом core; токен — CONTENT_API_TOKEN")
}

func TestLD_R12_S6_LoaderCallsRecheck(t *testing.T) {
	t.Skip("ВОПРОС: в deploy/secrets.md нет имени переменной с адресом core; токен — CONTENT_API_TOKEN")
}

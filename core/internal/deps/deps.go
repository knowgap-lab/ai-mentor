// Пакет deps удерживает библиотеки каркаса в go.mod, пока их не импортирует код продукта.
package deps

import (
	_ "github.com/jackc/pgx/v5"
	_ "github.com/pressly/goose/v3"
)

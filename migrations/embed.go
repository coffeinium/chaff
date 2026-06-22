// Пакет migrations вшивает нумерованные SQL-файлы схемы в бинарь. Применяются
// по порядку имён; применённую миграцию не правим — добавляем новую.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

## 1. Backend: сырые оценки в карточке эпика PDF

- [x] 1.1 В `internal/report/generator.go` добавить поле `RawTotalScore float64` в `epicTemplateData` и вычислить его в `buildReportTemplateData` как сумму значений `e.RawRoleScores` (по аналогии с уже существующим вычислением `RoundedTotal` для главной матрицы), проверить `go build ./...`
- [x] 1.2 В `config/template.html`, секция «Оценки по ролям» карточки эпика: переключить `{{range $role, $score := .RoleScores}}` на `{{range $role, $score := .RawRoleScores}}`; строку суммы заменить на `Сумма оценок (без рисков)` со значением `.RawTotalScore` (не `.FinalScore`)
- [x] 1.3 Убедиться, что шапка карточки (`Итого: {{.FinalScore}}`) и низ карточки (`Итоговая оценка с учётом рисков: {{.FinalScore}}`) не изменились — сверить `git diff config/template.html` (или ручной просмотр, т.к. файл не в git) на предмет отсутствия правок вне секции «Оценки по ролям»

## 2. QA

- [x] 2.1 Обновить `TestTemplateHTML_RendersWithoutError` (`internal/report/generator_test.go`): инвертировать проверку на `"без рисков"` (теперь SHALL присутствовать), сохранить проверку на `"с учётом риска"` (SHALL присутствовать, низ карточки не менялся)
- [x] 2.2 В тестовой fixture того же теста задать эпик с риск-фактором ≠ 1 (`RawRoleScores`, сумма которых заметно отличается от `FinalScore` — например, сумма 100, `FinalScore` 60) и явно проверить в отрендеренном HTML присутствие обоих чисел раздельно (сумма «без рисков» ≠ «Итого»/«Итоговая оценка с учётом рисков»), чтобы тест не проходил случайно при равных числах
- [x] 2.3 Прогнать `go build ./...`, `go vet ./...`, `go test ./...` — всё должно быть зелёным; убедиться, что `TestCapacityReport_XLSXAndPDFHTMLMatchForSamePeriod` (из `simplify-capacity-report`) по-прежнему проходит и не завязан на секцию карточки эпика, которую меняет эта задача

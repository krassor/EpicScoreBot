package handlers

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	tgbot "github.com/go-telegram/bot"
)

// maxExportImageBytes — верхний предел размера картинки диаграммы Ганта,
// принимаемой ExportGanttImage: собственный лимит Telegram Bot API на
// загрузку документа ботом (design.md Решение 7 заявки
// export-gantt-chart-image). Тело запроса режется http.MaxBytesReader —
// клиент дополнительно проверяет размер до отправки (см. frontend §6.1), но
// сервер не доверяет этой проверке.
const maxExportImageBytes = 50 * 1024 * 1024

// maxExportImageMemory — порог ParseMultipartForm, свыше которого части
// формы спуливаются во временный файл вместо памяти. Меньше
// maxExportImageBytes, чтобы typical (небольшая) картинка обрабатывалась в
// памяти, а редкий файл у самого предела не раздувал процесс.
const maxExportImageMemory = 10 * 1024 * 1024

// pngSignature — первые 8 байт любого валидного PNG-файла.
var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// utf8BOM — маркер порядка байт, который некоторые сериализаторы ставят
// перед XML/SVG-документом.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ExportGanttImage — POST /api/gantt/export/image. Принимает собранную на
// клиенте картинку диаграммы Ганта (PNG или SVG, см. frontend §3.1–3.6,
// design.md Решение 1) и пересылает её документом в личный чат Telegram
// пользователя, отправившего запрос (design.md Решение 7 заявки
// export-gantt-chart-image). Сервер ничего не парсит и не рендерит —
// только минимально проверяет содержимое (сигнатуру) и пересылает байты.
//
// Адресат берётся ИСКЛЮЧИТЕЛЬНО из сессии (session.TelegramID → chat_id
// пользователя в БД), никогда из тела запроса — это снимает вопрос о
// поверхности атаки: худшее, чего добивается злоумышленник, — отправка
// произвольного файла самому себе.
func (h *GanttHandler) ExportGanttImage(w http.ResponseWriter, r *http.Request) {
	op := "handlers.ExportGanttImage"
	log := h.log.With(slog.String("op", op))

	session, ok := requireSession(w, r)
	if !ok {
		return
	}

	// Telegram-бот мог не инициализироваться (см. app/main.go) — доставка
	// картинки недоступна независимо от прав отправителя, тот же код и тот
	// же приём, что и в NotifyEpicReminders (notify.go).
	if h.docSender == nil {
		writeErrorCode(w, http.StatusInternalServerError, "NOTIFIER_UNAVAILABLE",
			"telegram notifications are unavailable")
		return
	}

	// Тело режется на уровне net/http — размер файла проверяется здесь, а
	// не после полного чтения в память.
	r.Body = http.MaxBytesReader(w, r.Body, maxExportImageBytes)
	if err := r.ParseMultipartForm(maxExportImageMemory); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
				"file exceeds the 50MB limit")
			return
		}
		writeErrorCode(w, http.StatusBadRequest, "INVALID_REQUEST_BODY", "invalid multipart form")
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll() //nolint:errcheck
	}

	format := strings.ToLower(strings.TrimSpace(r.FormValue("format")))
	if format != "png" && format != "svg" {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_FORMAT", "format must be 'png' or 'svg'")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_FILE", "file is required")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeErrorCode(w, http.StatusRequestEntityTooLarge, "FILE_TOO_LARGE",
				"file exceeds the 50MB limit")
			return
		}
		writeErrorCode(w, http.StatusBadRequest, "INVALID_FILE", "failed to read file")
		return
	}

	// Минимальная проверка содержимого — не разбор и не рендер, только
	// защита от использования эндпоинта как пересылки произвольных файлов
	// от имени бота (design.md Решение 7).
	if !contentMatchesFormat(data, format) {
		writeErrorCode(w, http.StatusBadRequest, "INVALID_FILE", "file content does not match format")
		return
	}

	filename := sanitizeExportImageFilename(r.FormValue("filename"), format)

	// Адресат — ТОЛЬКО из сессии, не из тела запроса.
	// Отсутствие пользователя в БД (sql.ErrNoRows) — то же состояние, что и
	// нулевой chat_id: боту некуда писать. Прочие ошибки БД — сбой сервера,
	// а не действие пользователя: подсказка «откройте чат» здесь назвала бы
	// неверную причину.
	user, err := h.repo.FindUserByTelegramID(r.Context(), session.TelegramID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Error("failed to find user", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusInternalServerError, "internal_error", "failed to resolve recipient")
		return
	}
	if err != nil || user == nil || user.ChatID == 0 {
		writeErrorCode(w, http.StatusConflict, "BOT_CHAT_NOT_STARTED",
			"откройте личный чат с ботом в Telegram, чтобы получать файлы")
		return
	}

	const caption = "📊 Диаграмма Ганта"
	if err := h.docSender.SendDocumentToChat(r.Context(), user.ChatID, filename, data, caption); err != nil {
		if errors.Is(err, tgbot.ErrorForbidden) {
			writeErrorCode(w, http.StatusConflict, "BOT_CHAT_NOT_STARTED",
				"откройте личный чат с ботом в Telegram, чтобы получать файлы")
			return
		}
		log.Error("failed to send gantt chart image", slog.String("error", err.Error()))
		writeErrorCode(w, http.StatusBadGateway, "TELEGRAM_SEND_FAILED", "failed to send image to telegram")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

// contentMatchesFormat сообщает, соответствуют ли первые байты содержимого
// заявленному формату — сигнатура PNG либо начало SVG-документа (`<?xml`
// или `<svg`, после отбрасывания BOM и пробельных символов).
func contentMatchesFormat(data []byte, format string) bool {
	switch format {
	case "png":
		return bytes.HasPrefix(data, pngSignature)
	case "svg":
		trimmed := bytes.TrimPrefix(data, utf8BOM)
		trimmed = bytes.TrimLeft(trimmed, " \t\r\n")
		return bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<svg"))
	default:
		return false
	}
}

// isSafeFilenameRune сообщает, безопасен ли символ для имени файла: буквы
// (в т.ч. кириллица — имена команд не латинские, см. buildExportFileName на
// фронтенде) и цифры любого алфавита, плюс дефис/подчёркивание/точка/
// пробел. Разделители пути, управляющие символы и кавычки не входят.
func isSafeFilenameRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return true
	}
	switch r {
	case '-', '_', '.', ' ':
		return true
	}
	return false
}

// sanitizeExportImageFilename оставляет в имени файла, присланном клиентом,
// только безопасные символы (design.md Решение 7: «сервер оставляет в
// имени безопасные символы»), отбрасывая, а не заменяя, всё остальное. При
// пустом результате подставляет запасное имя. Расширение всегда приводится
// к актуальному format — переданное клиентом (если совпало с одним из двух
// допустимых) сперва срезается, чтобы не задвоилось.
func sanitizeExportImageFilename(raw, format string) string {
	var b strings.Builder
	for _, r := range raw {
		if isSafeFilenameRune(r) {
			b.WriteRune(r)
		}
	}
	name := strings.TrimSpace(b.String())
	name = strings.TrimSuffix(name, ".png")
	name = strings.TrimSuffix(name, ".svg")
	name = strings.TrimSpace(name)
	if name == "" {
		name = "gantt-chart"
	}
	return name + "." + format
}

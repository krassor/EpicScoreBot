package middleware

import "testing"

// TestUserSession_DirectoryKey проверяет нормализацию Username в ключ
// опознания справочника (design.md, Решение 2 заявки
// fix-webapp-user-identity): нижний регистр, срез ведущего "@", trim
// пробелов; пустая строка при незаданном username.
func TestUserSession_DirectoryKey(t *testing.T) {
	tests := []struct {
		name     string
		username string
		want     string
	}{
		{name: "ведущий @ и заглавная буква", username: "@Ivan", want: "ivan"},
		{name: "уже нормализовано", username: "ivan", want: "ivan"},
		{name: "пробелы по краям и верхний регистр", username: " IVAN ", want: "ivan"},
		{name: "username не задан", username: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := UserSession{Username: tt.username}
			if got := session.DirectoryKey(); got != tt.want {
				t.Errorf("DirectoryKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

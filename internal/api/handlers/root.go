// internal/api/handlers/root.go

package handlers

import (
	"fmt"
	"max-bot-service/internal/bot"
	"net/http"
)

type RootHandler struct {
	Bot *bot.Model
}

func (handler *RootHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	// MAX использует формат https://max.ru/@botname
	http.Redirect(writer, request, fmt.Sprintf("https://max.ru/ID%s", "1657113595_bot"), http.StatusSeeOther)
}

package api

import (
	"net/http"
)

// HandleChannelQRCode 获取渠道二维码（GET）
func HandleChannelQRCode(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	channel := r.URL.Query().Get("channel")
	if channel == "" {
		writeError(rw, http.StatusBadRequest, "channel parameter required")
		return
	}

	result, err := qrcodeSvc.FetchQRCode(channel)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, result)
}

// HandleChannelQRCodeStatus 轮询二维码扫描状态（GET）
func HandleChannelQRCodeStatus(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(rw, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	channel := r.URL.Query().Get("channel")
	token := r.URL.Query().Get("token")

	if channel == "" {
		writeError(rw, http.StatusBadRequest, "channel parameter required")
		return
	}
	if token == "" {
		writeError(rw, http.StatusBadRequest, "token parameter required")
		return
	}

	result, err := qrcodeSvc.PollQRCodeStatus(channel, token)
	if err != nil {
		writeError(rw, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(rw, http.StatusOK, result)
}
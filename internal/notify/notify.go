// Caminho: catraca/internal/notify/notify.go

// Package notify mostra a notificação do Windows (canto da tela, por cima de
// qualquer programa). Usa a mesma biblioteca que o Wails já traz.
package notify

import (
	"sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
)

const (
	appName = "PDV Academia"
	// appGUID identifica o programa no registro do usuário. Fixo: não trocar.
	appGUID = "{AE229AB7-9D39-44EF-B16F-E73E909E2B26}"
)

var once sync.Once

// Show exibe a notificação; icon é o caminho de uma imagem (vazio = sem foto).
func Show(title, body, icon string) error {
	once.Do(func() { _ = toast.SetAppData(toast.AppData{AppID: appName, GUID: appGUID}) })
	n := toast.Notification{AppID: appName, Title: title, Body: body, Icon: icon, IconCrop: toast.CropStyleCircle}
	return n.Push()
}

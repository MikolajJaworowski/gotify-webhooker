package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/go-playground/validator"
	"github.com/gomarkdown/markdown"
	"github.com/gorilla/websocket"
	"github.com/gotify/plugin-api"
)

type WebhookPost struct {
	Username	string	`json:"username"`
	Text			string	`json:"text"`
	Html			string	`json:"html"`
}

func GetGotifyPluginInfo() plugin.Info {
	return plugin.Info{
		ModulePath:  "github.com/gotify/plugin-template",
		Version:     "1.0.0",
		Author:      "KiMi",
		Website:     "",
		Description: "Plugin for forwarding messages to webhook",
		License:     "MIT",
		Name:        "WebhookerPlugin",
	}
}

type Config struct {
	WebhookUrl		string
	HostServer		string
	ClientToken		string
}

type Storage struct {
	WasEnabled bool `json:"wasEnabled"`
}

type WebhookerPlugin struct {
	enabled					bool
	storageHandler	plugin.StorageHandler
	config					*Config
}

func (c *WebhookerPlugin) SendPostToWebhook(webhookUrl string, message plugin.Message) (err error) {
	webhookPost := &WebhookPost{
		Username: message.Title,
		Text: message.Message,
		Html: string(markdown.ToHTML([]byte(message.Message), nil, nil)),
	}

	body, err := json.Marshal(webhookPost)

	log.Println("Sending: ", webhookPost)

	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", webhookUrl, bytes.NewBuffer(body))

	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return err
	}

	res.Body.Close()

	return
}

func (c *WebhookerPlugin) TestSocket(url string) (err error) {
	_, _, err = websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		log.Println("Test dial error : ", err)
		return  err
	}
	return nil
}

func (c *WebhookerPlugin) SetStorageHandler(h plugin.StorageHandler) {
    c.storageHandler = h
}

func (c *WebhookerPlugin) DefaultConfig() interface{} {
	config := &Config{
		WebhookUrl: "",
		ClientToken: "",
		HostServer: "ws://localhost:8080",
	}
	return config
}

func (c *WebhookerPlugin) ValidateAndSetConfig(conf interface{}) error {
    config := conf.(*Config)

		v := validator.New()
		err := v.Struct(config)

    if err != nil {
			log.Println("Validation error: ", err)
        return errors.New("Validation error: " + err.Error())
    }

    c.config = config

		storage := new(Storage)
		storageBytes, err := c.storageHandler.Load()

		if err != nil {
			return err
		}

		json.Unmarshal(storageBytes, storage)
		c.enabled = storage.WasEnabled

    return nil
}

func (c *WebhookerPlugin) Enable() error {
	if len (c.config.HostServer) < 1 {
		return errors.New("Please enter the correct web server")
	}
	if len (c.config.ClientToken) < 1 {
			return errors.New("Please enter the client token")
	}
	if len (c.config.WebhookUrl) < 1 {
		return errors.New("Please enter the correct webhook url")
	}

	serverUrl := c.config.HostServer + "/stream?token=" + c.config.ClientToken

	err := c.TestSocket(serverUrl)

	if err != nil {
		return errors.New("Web server url or client_token is not valid")
	}

	log.Println("Websocket url : ", serverUrl)

	go c.StartListener(serverUrl)

	c.enabled = true
	log.Println("Webhooker plugin enabled")

	storage := new(Storage)
	storageBytes, err := c.storageHandler.Load()

	if err != nil {
		return err
	}

	storage.WasEnabled = true
	storageBytes, _ = json.Marshal(storage)
	c.storageHandler.Save(storageBytes)

	return nil
}

func (c *WebhookerPlugin) Disable() error {
	c.enabled = false
	log.Println("Webhooker plugin disabled")

	storage := new(Storage)
	storageBytes, err := c.storageHandler.Load()

	if err != nil {
		return err
	}

	storage.WasEnabled = false
	storageBytes, _ = json.Marshal(storage)
	c.storageHandler.Save(storageBytes)

	return nil
}

func (c *WebhookerPlugin) StartListener(serverUrl string) (err error) {
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	ws, _, err := websocket.DefaultDialer.Dial(serverUrl, nil)

	if err != nil {
		log.Fatal("Websocket error: ", err)

		return err
	}

	log.Printf("Connected to %s", serverUrl)

	defer ws.Close()

	done := make(chan struct {})

	incomingMsg := plugin.Message{}

	go func() {
		defer close(done)

		for {
			_, message, err := ws.ReadMessage()

			if err != nil {
				log.Fatal("Websocket read message error: ", err)
				return
			}

			if err := json.Unmarshal(message, &incomingMsg); err != nil {
				log.Fatal("Json parsing error: ", err)
			}

			err = c.SendPostToWebhook(c.config.WebhookUrl, incomingMsg)

			if err != nil {
				log.Printf("POST error: %v", err)
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <- done:
			return
		case t := <- ticker.C:
			err := ws.WriteMessage(websocket.TextMessage, []byte(t.String()))

			if err != nil {
				log.Println("Websocket write error: ", err)

				return err
			}
		case <- interrupt:
			log.Println("Interrupt received")

			err := ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))

			if err != nil {
				log.Println("Websocket close error: ", err)
				return err
			}

			select {
			case <- done:
			case <- time.After(time.Second):
			}
			return err
		}
	}
}

func NewGotifyPluginInstance(ctx plugin.UserContext) plugin.Plugin {
	return &WebhookerPlugin{}
}

func main() {
	panic("this should be built as go plugin")
}

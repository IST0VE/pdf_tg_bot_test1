package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	wkhtml "github.com/SebastiaanKlippert/go-wkhtmltopdf"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/joho/godotenv"
)

type Prescription struct {
	Lpu         string
	Discount    string
	Seria       string
	Number      string
	Date        string
	ValidUntil  string
	ExpPeriod   string
	DoctorFio   string
	Medicine    string
	Medform     string
	Dose        string
	DoseMeasure string
	PackNumb    string
	PackCount   string
	UseMethod   string
}

var tmpl = `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <title>Рецепт</title>
    <style>
        body { font-family: 'Arial', sans-serif; margin: 0; padding: 0; background-color: #f4f4f4; }
        .container { width: 100%; max-width: 800px; margin: 30px auto; padding: 20px; background-color: #fff; border-radius: 10px; box-shadow: 0 4px 8px rgba(0,0,0,0.2); }
        h1 { color: #333; margin-bottom: 20px; }
        .field { margin-bottom: 12px; padding: 8px; background-color: #f9f9f9; border-left: 5px solid #007BFF; }
        .label { font-weight: bold; display: block; margin-bottom: 5px; }
        .value { margin-left: 5px; }
        .highlight { color: #007BFF; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Рецепт: <span class="highlight">{{.Medicine}}</span></h1>

		{{if or (notNull .Seria) (notNull .Number)}}
		<div class="field"><span class="label">Серия и номер:</span><span class="value">{{.Seria}} {{.Number}}</span></div>
		{{end}}

        <div class="field"><span class="label">Лечебное учреждение:</span><span class="value">{{.Lpu}}</span></div>
        <div class="field"><span class="label">ФИО врача:</span><span class="value">{{.DoctorFio}}</span></div>
        <div class="field"><span class="label">Дата выдачи:</span><span class="value">{{.Date}}</span></div>
        <div class="field"><span class="label">Срок годности:</span><span class="value">{{.ExpPeriod}}</span></div>
		<div class="field"><span class="label">Действует до:</span><span class="value">{{.ValidUntil}}</span></div>
        <div class="field"><span class="label">Лекарственная форма:</span><span class="value">{{.Medform}}</span></div>
        <div class="field"><span class="label">Дозировка:</span><span class="value">{{.Dose}}</span></div>
        <div class="field"><span class="label">Количество упаковок:</span><span class="value">{{.PackNumb}}</span></div>
        <div class="field"><span class="label">Метод применения:</span><span class="value">{{.UseMethod}}</span></div>
        <div class="field"><span class="label">Статус льготы:</span><span class="value">{{if eq .Discount "2"}}Коммерческий{{else if eq .Discount "1"}}Льготный{{else}}Нельготный{{end}}</span></div>
    </div>
</body>
</html>`

func main() {

	if err := godotenv.Load(); err != nil {
		log.Fatalf("Error loading .env file: %v", err)
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is not set")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = true

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			switch update.Message.Text {
			case "/start":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Привет! Отправь мне JSON с данными рецепта.")
				bot.Send(msg)

			default:
				var prescription Prescription
				err := json.Unmarshal([]byte(update.Message.Text), &prescription)
				if err != nil {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "Ошибка в формате данных. Пожалуйста, отправьте корректный JSON.")
					bot.Send(msg)
					continue
				}

				startTime := time.Now() // Время старта обработки JSON

				prescription.ValidUntil, err = calculateValidity(prescription.Date, prescription.ExpPeriod)
				if err != nil {
					log.Println("Ошибка при вычислении даты действия рецепта:", err)
					continue
				}

				// Генерация HTML из шаблона
				htmlOutput, err := generateHTMLFromTemplate(prescription)
				if err != nil {
					log.Println("Error generating HTML:", err)
					continue
				}

				// Генерация PDF
				pdfFilename, err := generatePDF(htmlOutput)
				if err != nil {
					log.Println("Error generating PDF:", err)
					continue
				}
				duration := time.Since(startTime) // Вычисление времени выполнения
				log.Printf("Обработка заняла: %v", duration)

				// Отправка PDF
				sendPDF(update.Message.Chat.ID, pdfFilename, bot)

				// Отправить сообщение о времени обработки
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("Обработка заняла: %v", duration))
				bot.Send(msg)
			}
		}
	}
}

func generateHTMLFromTemplate(data Prescription) (string, error) {
	t, err := template.New("webpage").Funcs(template.FuncMap{
		"notNull": notNull,
	}).Parse(tmpl)
	if err != nil {
		return "", err
	}

	var tplBuffer bytes.Buffer
	err = t.Execute(&tplBuffer, data)
	if err != nil {
		return "", err
	}

	return tplBuffer.String(), nil
}

func generatePDF(html string) (string, error) {
	pdfg, err := wkhtml.NewPDFGenerator()
	if err != nil {
		return "", err
	}

	pdfg.AddPage(wkhtml.NewPageReader(strings.NewReader(html)))

	err = pdfg.Create()
	if err != nil {
		return "", err
	}

	filename := "prescription.pdf"
	err = pdfg.WriteFile(filename)
	if err != nil {
		return "", err
	}

	return filename, nil
}

func sendPDF(chatID int64, filePath string, bot *tgbotapi.BotAPI) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FilePath(filePath))
	msg, err := bot.Send(doc)
	if err != nil {
		log.Printf("Error sending document: %s", err)
	} else {
		log.Printf("Document sent successfully: %v", msg.Document.FileID)
	}
}

func notNull(s string) bool {
	return s != "" && s != "null"
}

func calculateValidity(dateStr string, expPeriod string) (string, error) {
	layout := "02.01.2006" // формат даты
	parsedDate, err := time.Parse(layout, dateStr)
	if err != nil {
		return "", err
	}
	expDays, err := strconv.Atoi(strings.Fields(expPeriod)[0]) // извлекаем количество дней из строки
	if err != nil {
		return "", err
	}
	validUntil := parsedDate.AddDate(0, 0, expDays)
	return validUntil.Format(layout), nil
}

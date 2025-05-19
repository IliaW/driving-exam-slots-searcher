package main

import (
	"context"
	"errors"
	"fmt"
	"hsc-gov/config"
	"hsc-gov/model"
	"hsc-gov/utils"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
	"github.com/lmittmann/tint"
)

const (
	URL           = "https://eqn.hsc.gov.ua/cabinet/queue"
	TokenFileName = "secret.txt"
)

var (
	authorizationSelector       = "//p[contains(text(), 'Увійти за допомогою')]/.."
	signUpInTheQueueSelector    = "//h6[contains(text(), 'Записатись у чергу')]/.."
	theoreticalExamSelector     = "//p[contains(text(), 'Теоретичний іспит ')]/.."
	practicalExamSelector       = "//p[contains(text(), 'Практичний іспит')]/.."
	mvsCarSelector              = "//p[contains(text(), 'Практичний іспит на транспортному засобі Сервісного центру МВС')]/.."
	categoryBMechanicalSelector = "//p[contains(text(), 'категорія В (механічна КПП)')]/.."
	dateSelector                = "//button[not(@disabled)]/abbr[text()='%v']/.."
	nextButtonSelector          = "//p[text()='Далі']/.."
	mapSelector                 = "//div[contains(@class, 'leaflet-container')]"
	addressSelector             = "//main//p[contains(text(),'%s')]/../../.."
	timeSlotsContainerSelector  = "//p[contains(text(), 'Доступний час')]"
)

var (
	cfg                    *config.Config
	canselFunc             context.CancelFunc
	defaultTimeout         time.Duration
	examType               int
	secureTokenCookieName  = "__Secure-next-auth.session-token"
	secureTokenCookieValue = ""
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	canselFunc = stop

	cfg = config.MustLoadConfig()
	examType = cfg.ExamType
	defaultTimeout = cfg.DefaultTimeout
	setupLogger()
	examDates := strings.Split(cfg.ExamDates, ";")
	addresses := strings.Split(cfg.Addresses, ";")
	taskList := createTasks(examDates, addresses)
	taskChan := make(chan *model.Task, len(taskList))
	refreshToken()

	utils.SendNotification(&model.Notification{
		Topic:   cfg.NtfyTopic,
		Title:   fmt.Sprintf("%v Start looking for exam slots!", utils.Emoji_Loudspeaker),
		Message: fmt.Sprintf("DATES: %v; CITIES: %v ", cfg.ExamDates, cfg.Addresses),
	})

	// TODO: catch panic from page.MustNavigate(URL).MustWaitLoad() and recover with new browser
	wg := &sync.WaitGroup{}
	once := &sync.Once{}
	for range getBrowserInstanceCount(taskList) {
		wg.Add(1)
		go runBrowser(taskChan, wg, once)
	}

	go taskTicker(ctx, taskChan, taskList)

	wg.Wait()
	utils.SendNotification(&model.Notification{
		Topic:   cfg.NtfyTopic,
		Title:   "Stop looking for exam slots!",
		Message: fmt.Sprintf(utils.Emoji_Loudspeaker),
	})

	slog.Info("done")
}

func getBrowserInstanceCount(taskList []*model.Task) int {
	browsersCount := cfg.BrowsersCount
	if len(taskList) < cfg.BrowsersCount {
		browsersCount = len(taskList)
	}

	return browsersCount
}

func taskTicker(ctx context.Context, taskChan chan *model.Task, taskList []*model.Task) {
	for _, task := range taskList {
		taskChan <- task
	}

	ticker := time.NewTicker(cfg.IntervalBetweenChecks)
	for {
		select {
		case <-ctx.Done():
			close(taskChan)
			slog.Info("close taskChan. Application shutdown...")
			return
		case <-ticker.C:
			for _, task := range taskList {
				taskChan <- task
			}
		}
	}
}

func runBrowser(taskChan chan *model.Task, wg *sync.WaitGroup, once *sync.Once) {
	defer wg.Done()

	l := launcher.New().
		Leakless(false).
		Headless(cfg.HeadlessBrowser).
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows")
	defer l.Cleanup()

	url := l.MustLaunch()
	browser := rod.New().
		ControlURL(url).
		MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser).MustWindowMaximize()
	defer page.MustClose()
	initSecureCookies(page)

	for task := range taskChan {
		if task.Found == true && time.Now().Sub(task.UpdatedAt) < task.Ttl {
			slog.Debug("task result not expired", slog.String("date", task.ExamDate),
				slog.String("address", task.Address))
			continue
		}

		task.Found = false
		page.MustNavigate(URL).MustWaitLoad()

		if isElementPresent(authorizationSelector, page, 1*time.Second) {
			once.Do(func() {
				refreshToken()
			})
			initSecureCookies(page)
			page.MustNavigate(URL).MustWaitLoad()
		}

		if isElementPresent(signUpInTheQueueSelector, page, 1*time.Second) {
			err := findAndClick(signUpInTheQueueSelector, page, defaultTimeout)
			if err != nil {
				slog.Error("can't click `Записатись у чергу` button", slog.String("error", err.Error()))
				continue
			}
		}

		// Select exam type
		switch examType {
		case 0:
			err := findAndClick(theoreticalExamSelector, page, defaultTimeout)
			if err != nil {
				slog.Error("can't click `Теоретичний іспит`", slog.String("error", err.Error()))
				continue
			}
		case 1:
			err := findAndClick(practicalExamSelector, page, defaultTimeout)
			if err != nil {
				slog.Error("can't click `Практичний іспит`", slog.String("error", err.Error()))
				continue
			}
			err = findAndClick(mvsCarSelector, page, defaultTimeout)
			if err != nil {
				slog.Error("can't click `Практичний іспит на транспортному засобі Сервісного центру МВС`",
					slog.String("error", err.Error()))
				continue
			}
			err = findAndClick(categoryBMechanicalSelector, page, defaultTimeout)
			if err != nil {
				slog.Error("can't click `категорія В (механічна КПП)`", slog.String("error", err.Error()))
				continue
			}
		}

		// Select exam date
		err := findAndClick(fmt.Sprintf(dateSelector, task.ExamDate), page, defaultTimeout)
		if err != nil {
			slog.Error(fmt.Sprintf("can't select exam date %v", task.ExamDate),
				slog.String("error", err.Error()))
			continue
		}
		err = findAndClick(nextButtonSelector, page, defaultTimeout)
		if err != nil {
			slog.Error("can't click `Далі`", slog.String("error", err.Error()))
			continue
		}

		// Wait for map display to continue
		err = findElement(mapSelector, page, 15*time.Second)
		if err != nil {
			slog.Error("can't find map", slog.String("error", err.Error()),
				slog.String("examDate", task.ExamDate), slog.String("address", task.Address))
			continue
		}

		// Select City
		err = findAndClick(fmt.Sprintf(addressSelector, task.Address), page, defaultTimeout)
		if err != nil {
			// most possible the address is not available. Not an error
			slog.Debug("can't find address", slog.String("error", err.Error()))
			continue
		}

		err = findAndClick(nextButtonSelector, page, defaultTimeout)
		if err != nil {
			slog.Error("can't click `Далі`", slog.String("error", err.Error()))
			continue
		}

		if isElementPresent(timeSlotsContainerSelector, page, defaultTimeout) {
			message := fmt.Sprintf("%v DATE: %v; CITY: %v ", utils.Emoji_Tada, task.ExamDate, task.Address)
			tempFilePath := takeScreenshot(page)
			utils.SendNotification(&model.Notification{
				Topic:    cfg.NtfyTopic,
				Title:    message,
				Filename: tempFilePath,
			})
			_ = os.Remove(tempFilePath)
			task.Found = true
			task.UpdatedAt = time.Now()
			slog.Info("found time slot", slog.String("info", message))
		} else {
			// sometimes an address exists but no time slots available
			slog.Info(fmt.Sprintf("can't find time slots for DATE: %v CITY: %v", task.ExamDate, task.Address))
			continue
		}
	}
}

func takeScreenshot(page *rod.Page) string {
	screenshotBytes, _ := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	tempFile, err := os.CreateTemp("", "screenshot-*.png")
	if err != nil {
		slog.Error("can't create temp file", slog.String("error", err.Error()))
		return ""
	}
	defer tempFile.Close()

	_, err = tempFile.Write(screenshotBytes)
	if err != nil {
		slog.Error("can't write to temp file", slog.String("error", err.Error()))
		return ""
	}

	return tempFile.Name()
}

func refreshToken() {
	slog.Info("refreshing token")
	data, err := os.ReadFile(TokenFileName)
	if err != nil {
		fatalErr("ERR: can't read token file", err)
	}
	secureTokenCookieValue = string(data)

	l := launcher.New().
		Leakless(false).
		Headless(false).
		Set("disable-background-timer-throttling").
		Set("disable-backgrounding-occluded-windows")
	defer l.Cleanup()
	url := l.MustLaunch()

	browser := rod.New().
		ControlURL(url).
		MustConnect()
	defer browser.MustClose()

	page := stealth.MustPage(browser).MustWindowMaximize()
	defer page.MustClose()
	initSecureCookies(page)
	page.MustNavigate(URL).MustWaitLoad()

	if isElementPresent(authorizationSelector, page, 1*time.Second) {
		utils.SendNotification(&model.Notification{
			Topic:    cfg.NtfyTopic,
			Title:    fmt.Sprintf("%v%v Authorization required!", utils.Emoji_Warning, utils.Emoji_Loudspeaker),
			Message:  "Pass authorization in the opened browser window within 10 minutes.",
			Priority: 4,
		})
		err := findElement(signUpInTheQueueSelector, page, 10*time.Minute)
		if err != nil {
			fatalErr("Authorization failed", err)
		}

		newToken, err := findCookieValue(page, secureTokenCookieName)
		if err != nil {
			fatalErr("ERR: can't find token", err)
		}
		slog.Info("new token found")

		writeErr := os.WriteFile(TokenFileName, []byte(newToken), 0644)
		if writeErr != nil {
			slog.Error("can't save token to file", writeErr)
		}
		secureTokenCookieValue = newToken
		return
	}
	slog.Info("saved token still valid")
}

func createTasks(dates []string, addresses []string) []*model.Task {
	tasks := make([]*model.Task, 0, len(dates)*len(addresses))
	for _, date := range dates {
		for _, addr := range addresses {
			task := &model.Task{
				ExamDate: strings.TrimSpace(date),
				Address:  strings.TrimSpace(addr),
				Found:    false,
				Ttl:      cfg.TtlForFoundTask,
			}
			tasks = append(tasks, task)
		}
	}

	return tasks
}

func initSecureCookies(page *rod.Page) {
	cookieParam1 := &proto.NetworkCookieParam{
		Name:     secureTokenCookieName,
		Value:    secureTokenCookieValue,
		Domain:   "eqn.hsc.gov.ua",
		Path:     "/",
		Secure:   true,
		HTTPOnly: true,
	}
	err := page.SetCookies([]*proto.NetworkCookieParam{cookieParam1})
	if err != nil {
		fatalErr("can't set secure cookies", err)
	}
}

func findCookieValue(page *rod.Page, cookieName string) (string, error) {
	cookies, err := page.Cookies(nil)
	if err != nil {
		return "", err
	}

	cookieValue := ""
	for _, cookie := range cookies {
		if cookie.Name == cookieName {
			cookieValue = cookie.Value
			break
		}
	}
	if cookieValue == "" {
		return "", errors.New(fmt.Sprintf("cookie with name %s not Found", cookieName))
	}

	return cookieValue, nil
}

func isElementPresent(selector string, page *rod.Page, waitFor time.Duration) bool {
	if err := rod.Try(func() {
		page.Timeout(waitFor).MustElementX(selector).MustWaitVisible()
	}); err != nil {
		return false
	}

	return true
}

func findElement(selector string, page *rod.Page, waitFor time.Duration) error {
	return rod.Try(func() {
		page.Timeout(waitFor).MustElementX(selector).MustWaitVisible()
	})
}

func findAndClick(selector string, page *rod.Page, waitFor time.Duration) error {
	return rod.Try(func() {
		page.Timeout(waitFor).MustElementX(selector).MustWaitVisible().MustClick()
	})
}

func fatalErr(message string, err error) {
	slog.Error(message, slog.Any("error", err))
	utils.SendNotification(&model.Notification{
		Topic:   cfg.NtfyTopic,
		Title:   fmt.Sprintf("%v%v Something went wrong", utils.Emoji_Warning, utils.Emoji_Facepalm),
		Message: message,
	})
	os.Exit(1)
}

func setupLogger() *slog.Logger {
	envLogLevel := strings.ToLower(cfg.LogLevel)
	var slogLevel slog.Level
	err := slogLevel.UnmarshalText([]byte(envLogLevel))
	if err != nil {
		log.Printf("encountenred log level: '%s'. The package does not support custom log levels", envLogLevel)
		slogLevel = slog.LevelDebug
	}
	log.Printf("slog level overwritten to '%v'", slogLevel)
	slog.SetLogLoggerLevel(slogLevel)

	replaceAttrs := func(groups []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			source := a.Value.Any().(*slog.Source)
			source.File = filepath.Base(source.File)
		}
		return a
	}

	logger := slog.New(tint.NewHandler(os.Stdout, &tint.Options{
		AddSource:   true,
		Level:       slogLevel,
		ReplaceAttr: replaceAttrs,
		NoColor:     false,
	}))

	slog.SetDefault(logger)
	logger.Debug("debug messages are enabled")

	return logger
}

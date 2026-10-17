package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/atotto/clipboard"
	"github.com/fatih/color"
	"github.com/mdp/qrterminal/v3"
	"github.com/schollz/progressbar/v3"
	"github.com/urfave/cli/v2"
)

var Version = "dev"

// 🆕 Современный HTML с Drag & Drop и Fetch API
const uploadHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>QuickDrop Upload</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; text-align: center; padding: 40px 20px; background: #f0f2f5; color: #333; }
        .container { background: white; padding: 30px; border-radius: 12px; box-shadow: 0 4px 12px rgba(0,0,0,0.08); max-width: 400px; margin: 0 auto; }
        h2 { margin-top: 0; color: #1a1a1a; }
        
        /* Стили для зоны Drag & Drop */
        #drop-zone { 
            border: 2px dashed #ccc; 
            border-radius: 12px; 
            padding: 40px 20px; 
            margin: 20px 0; 
            transition: all 0.3s ease; 
            cursor: pointer;
            background: #fafafa;
        }
        #drop-zone.hover { 
            border-color: #007bff; 
            background: #e7f1ff; 
            transform: scale(1.02);
        }
        #drop-zone p { margin: 0; color: #666; font-size: 14px; }
        #drop-zone .icon { font-size: 40px; margin-bottom: 10px; }
        
        #status { margin-top: 20px; font-weight: bold; min-height: 20px; }
        .success { color: #28a745; }
        .error { color: #dc3545; }
        .loading { color: #007bff; }
    </style>
</head>
<body>
    <div class="container">
        <h2>📤 Отправить файл</h2>
        
        <div id="drop-zone">
            <div class="icon">📁</div>
            <p>Перетащи файл сюда<br>или нажми для выбора</p>
        </div>
        <input type="file" id="file-input" hidden>
        
        <div id="status"></div>
    </div>

    <script>
        const dropZone = document.getElementById('drop-zone');
        const fileInput = document.getElementById('file-input');
        const status = document.getElementById('status');

        // Клик по зоне открывает выбор файла
        dropZone.addEventListener('click', () => fileInput.click());

        // Обработка Drag & Drop
        dropZone.addEventListener('dragover', (e) => {
            e.preventDefault();
            dropZone.classList.add('hover');
        });

        dropZone.addEventListener('dragleave', () => {
            dropZone.classList.remove('hover');
        });

        dropZone.addEventListener('drop', (e) => {
            e.preventDefault();
            dropZone.classList.remove('hover');
            if (e.dataTransfer.files.length) {
                uploadFile(e.dataTransfer.files[0]);
            }
        });

        // Обработка выбора через клик
        fileInput.addEventListener('change', (e) => {
            if (e.target.files.length) {
                uploadFile(e.target.files[0]);
            }
        });

        // Функция отправки файла через Fetch API (без перезагрузки страницы)
        function uploadFile(file) {
            const formData = new FormData();
            formData.append('file', file);

            status.innerHTML = '<span class="loading">⏳ Загрузка...</span>';
            dropZone.style.pointerEvents = 'none'; // Блокируем повторные клики

            fetch('/upload', {
                method: 'POST',
                body: formData
            })
            .then(response => {
                if (response.ok) {
                    status.innerHTML = '<span class="success">✅ Файл успешно отправлен!</span>';
                    dropZone.style.display = 'none'; // Скрываем зону после успеха
                } else {
                    status.innerHTML = '<span class="error">❌ Ошибка при загрузке</span>';
                    dropZone.style.pointerEvents = 'auto';
                }
            })
            .catch(err => {
                status.innerHTML = '<span class="error">❌ Ошибка сети</span>';
                dropZone.style.pointerEvents = 'auto';
            });
        }
    </script>
</body>
</html>`

func main() {
	infoColor := color.New(color.FgCyan, color.Bold)
	successColor := color.New(color.FgGreen, color.Bold)
	errorColor := color.New(color.FgRed, color.Bold)

	app := &cli.App{
		Name:    "quickdrop",
		Usage:   "Мгновенный обмен файлами между устройствами через QR-код",
		Version: Version,
		Commands: []*cli.Command{
			{
				Name:      "send",
				Usage:     "Отправить файл или папку",
				ArgsUsage: "<путь>",
				Action: func(c *cli.Context) error {
					if c.NArg() < 1 {
						errorColor.Println("Ошибка: не указан путь к файлу или папке")
						return cli.Exit("Использование: quickdrop send <путь>", 1)
					}
					return sendFile(c.Args().First(), infoColor, successColor, errorColor)
				},
			},
			{
				Name:  "receive",
				Usage: "Принять файл с телефона",
				Action: func(c *cli.Context) error {
					return receiveFile(infoColor, successColor, errorColor)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func sendFile(filePath string, info, success, error *color.Color) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		error.Printf("Ошибка пути: %v\n", err)
		return err
	}

	fileInfo, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		error.Printf("Не найдено: %s\n", absPath)
		return err
	}

	finalFilePath := absPath
	finalFileName := filepath.Base(absPath)
	var tempZipPath string
	var fileSize int64

	if fileInfo.IsDir() {
		info.Println("📁 Обнаружена папка. Упаковываем в ZIP...")

		tmpFile, err := os.CreateTemp("", "quickdrop-*.zip")
		if err != nil {
			error.Printf("Не удалось создать временный файл: %v\n", err)
			return err
		}
		tmpFile.Close()
		tempZipPath = tmpFile.Name()
		finalFilePath = tempZipPath
		finalFileName = finalFileName + ".zip"

		if err := zipDir(absPath, tempZipPath); err != nil {
			os.Remove(tempZipPath)
			error.Printf("Ошибка архивации: %v\n", err)
			return err
		}
		info.Printf("✅ Папка упакована: %s\n", finalFileName)
	}

	fileInfo, err = os.Stat(finalFilePath)
	if err != nil {
		error.Printf("Не удалось получить размер файла: %v\n", err)
		return err
	}
	fileSize = fileInfo.Size()

	if tempZipPath != "" {
		defer os.Remove(tempZipPath)
	}

	port, err := getFreePort()
	if err != nil {
		error.Printf("Не удалось найти свободный порт: %v\n", err)
		return err
	}

	localIP, err := getLocalIP()
	if err != nil {
		error.Printf("Не удалось получить IP: %v\n", err)
		return err
	}

	url := fmt.Sprintf("http://%s:%d/download", localIP, port)

	// 🆕 Копируем URL в буфер обмена
	if err := clipboard.WriteAll(url); err == nil {
		success.Println("📋 Ссылка скопирована в буфер обмена!")
	}

	done := make(chan struct{})
	var once sync.Once

	http.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", finalFileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileSize))

		info.Printf("📥 Начинаем передачу: %s (%.2f MB)\n", finalFileName, float64(fileSize)/(1024*1024))

		file, err := os.Open(finalFilePath)
		if err != nil {
			http.Error(w, "Не удалось открыть файл", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		bar := progressbar.NewOptions64(
			fileSize,
			progressbar.OptionSetDescription("Отправка"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() { fmt.Println() }),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)

		written, err := io.Copy(io.MultiWriter(w, bar), file)
		if err != nil {
			error.Printf("\nОшибка передачи: %v\n", err)
			return
		}

		info.Printf("✅ Передано: %.2f MB\n", float64(written)/(1024*1024))
		once.Do(func() { close(done) })
	})

	printQR(url, info)
	success.Printf("✅ Сервер запущен! Ждем скачивания...\n")

	srv := &http.Server{Addr: fmt.Sprintf(":%d", port)}

	go func() {
		<-done
		success.Println("\n🎉 Файл успешно передан! Завершаем работу...")
		srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		error.Printf("Ошибка сервера: %v\n", err)
		return err
	}
	return nil
}

func receiveFile(info, success, error *color.Color) error {
	receiveDir := "./received"
	os.MkdirAll(receiveDir, os.ModePerm)

	port, err := getFreePort()
	if err != nil {
		error.Printf("Не удалось найти свободный порт: %v\n", err)
		return err
	}

	localIP, err := getLocalIP()
	if err != nil {
		error.Printf("Не удалось получить IP: %v\n", err)
		return err
	}

	url := fmt.Sprintf("http://%s:%d/", localIP, port)

	// 🆕 Копируем URL в буфер обмена
	if err := clipboard.WriteAll(url); err == nil {
		success.Println(" Ссылка скопирована в буфер обмена!")
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, uploadHTML)
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
		err := r.ParseMultipartForm(100 << 20)
		if err != nil {
			http.Error(w, "Файл слишком большой", http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Файл не выбран", http.StatusBadRequest)
			return
		}
		defer file.Close()

		info.Printf(" Получаем файл: %s (%.2f MB)\n", header.Filename, float64(header.Size)/(1024*1024))

		bar := progressbar.NewOptions64(
			header.Size,
			progressbar.OptionSetDescription("Прием"),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowCount(),
			progressbar.OptionOnCompletion(func() { fmt.Println() }),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)

		// 🆕 Умное имя файла (защита от перезаписи)
		dstPath := filepath.Join(receiveDir, header.Filename)
		dstPath = getSafePath(dstPath)

		dst, err := os.Create(dstPath)
		if err != nil {
			http.Error(w, "Ошибка сохранения", http.StatusInternalServerError)
			return
		}
		defer dst.Close()

		written, err := io.Copy(io.MultiWriter(dst, bar), file)
		if err != nil {
			error.Printf("\nОшибка сохранения: %v\n", err)
			return
		}

		info.Printf("✅ Сохранено как: %s (%.2f MB)\n", filepath.Base(dstPath), float64(written)/(1024*1024))

		// Возвращаем JSON или текст для JS
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, "OK")
	})

	printQR(url, info)
	success.Printf("✅ Режим приема включен! Файлы сохраняются в: %s\n", receiveDir)
	success.Printf("️  Нажми Ctrl+C для остановки.\n")

	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

// 🆕 Функция для защиты от перезаписи файлов
func getSafePath(dstPath string) string {
	if _, err := os.Stat(dstPath); os.IsNotExist(err) {
		return dstPath
	}

	ext := filepath.Ext(dstPath)
	base := strings.TrimSuffix(dstPath, ext)

	for i := 1; ; i++ {
		newPath := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}

func zipDir(source string, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	info, err := os.Stat(source)
	if err != nil {
		return err
	}

	var baseDir string
	if info.IsDir() {
		baseDir = filepath.Base(source)
	}

	return filepath.WalkDir(source, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
}

func getFreePort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("локальный IP не найден")
}

func printQR(url string, info *color.Color) {
	fmt.Println()
	info.Printf(" URL: %s\n", url)
	fmt.Println("📱 Отсканируй QR-код:")
	fmt.Println()
	qrterminal.Generate(url, qrterminal.L, os.Stdout)
	fmt.Println()
}

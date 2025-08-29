package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Experiment struct {
	Name        string
	Description string
	StartTime   time.Time
	EndTime     time.Time
	FileName    string
}

func main() {
	fmt.Println("=== Система управления экспериментами ===")
	fmt.Println("Хранение данных в текстовых файлах CSV формата")

	// Создание директории для экспериментов
	if err := createExperimentsDir(); err != nil {
		log.Fatal("Ошибка создания директории:", err)
	}

	// Запрос данных эксперимента
	experiment, err := getExperimentDetails()
	if err != nil {
		log.Fatal("Ошибка ввода:", err)
	}

	// Создание файла эксперимента
	file, err := createExperimentFile(experiment)
	if err != nil {
		log.Fatal("Ошибка создания файла:", err)
	}

	// Подключение к удаленному серверу
	conn, err := connectToRemoteServer()
	if err != nil {
		file.Close()
		log.Fatal("Ошибка подключения:", err)
	}

	fmt.Println("Подключение установлено. Начинаем сбор данных...")
	fmt.Println("Для остановки введите 'stop'")

	// Создание контекста для управления горутинами
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	dataChan := make(chan string, 100) // Буферизованный канал

	// Запуск сбора данных
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(dataChan) // Закрываем канал при завершении
		collectData(ctx, conn, dataChan)
	}()

	// Запуск сохранения данных
	wg.Add(1)
	go func() {
		defer wg.Done()
		saveDataToFile(ctx, file, dataChan)
	}()

	// Ожидание команды остановки
	waitForStopCommand(cancel)

	// Ожидаем завершения всех горутин
	wg.Wait()

	// Завершение эксперимента
	experiment.EndTime = time.Now()
	if err := finalizeExperiment(file, experiment); err != nil {
		log.Printf("Ошибка завершения эксперимента: %v", err)
	}

	// Закрываем файл и соединение
	file.Close()
	conn.Close()

	fmt.Println("Эксперимент завершен. Данные сохранены в:", experiment.FileName)
}

func createExperimentsDir() error {
	return os.MkdirAll("experiments", 0755)
}

func getExperimentDetails() (*Experiment, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Введите название эксперимента: ")
	name, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)

	fmt.Print("Введите описание эксперимента: ")
	description, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	description = strings.TrimSpace(description)

	// Генерация имени файла на основе времени и названия
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("experiments/%s_%s.csv",
		strings.ReplaceAll(name, " ", "_"), timestamp)

	return &Experiment{
		Name:        name,
		Description: description,
		StartTime:   time.Now(),
		FileName:    fileName,
	}, nil
}

func createExperimentFile(exp *Experiment) (*os.File, error) {
	// Создание всех необходимых директорий
	dir := filepath.Dir(exp.FileName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	file, err := os.Create(exp.FileName)
	if err != nil {
		return nil, err
	}

	// Запись метаданных в файл
	writer := bufio.NewWriter(file)

	// Первая строка - название
	writer.WriteString(fmt.Sprintf("Название эксперимента: %s\n", exp.Name))

	// Вторая строка - описание
	writer.WriteString(fmt.Sprintf("Описание: %s\n", exp.Description))

	// Третья строка - время начала
	writer.WriteString(fmt.Sprintf("Время начала: %s\n", exp.StartTime.Format("2006-01-02 15:04:05")))

	// Пустая строка разделитель
	writer.WriteString("\n")

	// Заголовок CSV
	writer.WriteString("timestamp\tvalue\n")

	if err := writer.Flush(); err != nil {
		file.Close()
		return nil, err
	}

	return file, nil
}

func connectToRemoteServer() (net.Conn, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Введите адрес сервера (host:port): ")
	address, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	address = strings.TrimSpace(address)

	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func collectData(ctx context.Context, conn net.Conn, dataChan chan<- string) {
	reader := bufio.NewReader(conn)

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Сбор данных остановлен")
			return
		default:
			// Устанавливаем таймаут на чтение
			conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

			data, err := reader.ReadString('\n')
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Таймаут - это нормально, продолжаем цикл
					continue
				}
				if strings.Contains(err.Error(), "use of closed network connection") {
					// Соединение закрыто - выходим
					return
				}
				log.Printf("Ошибка чтения данных: %v", err)
				return
			}

			data = strings.TrimSpace(data)
			if data == "" {
				continue
			}

			// Отправка данных в канал
			select {
			case dataChan <- data:
			case <-ctx.Done():
				return
			}
		}
	}
}

func saveDataToFile(ctx context.Context, file *os.File, dataChan <-chan string) {
	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataChan:
			if !ok {
				// Канал закрыт
				return
			}

			// Преобразование данных в CSV формат
			timestamp := time.Now().Format("2006-01-02 15:04:05")

			var csvLine string
			if !strings.Contains(data, ",") {
				csvLine = fmt.Sprintf("%s,raw_data,%s,\n", timestamp, data)
			} else {
				csvLine = data + "\n"
			}

			// Запись в файл
			if _, err := file.WriteString(csvLine); err != nil {
				log.Printf("Ошибка записи в файл: %v", err)
			} else {
				fmt.Printf("Сохранено: %s", csvLine)
			}

			// Синхронизация с диском
			file.Sync()
		}
	}
}

func waitForStopCommand(cancel context.CancelFunc) {
	reader := bufio.NewReader(os.Stdin)
	for {
		input, err := reader.ReadString('\n')
		if err != nil {
			log.Printf("Ошибка чтения ввода: %v", err)
			continue
		}

		if strings.TrimSpace(input) == "stop" {
			fmt.Println("Останавливаем эксперимент...")
			cancel()
			break
		}
	}
}

func finalizeExperiment(file *os.File, exp *Experiment) error {
	writer := bufio.NewWriter(file)

	// Запись времени окончания
	writer.WriteString(fmt.Sprintf("\nВремя окончания: %s\n",
		exp.EndTime.Format("2006-01-02 15:04:05")))

	writer.WriteString(fmt.Sprintf("Длительность эксперимента: %v\n",
		exp.EndTime.Sub(exp.StartTime)))

	return writer.Flush()
}

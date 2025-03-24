package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"go.bug.st/serial"
)

var (
	listenAddr     = flag.String("listen", ":8080", "Адрес прослушивания TCP-сервера")
	comPort        = flag.String("com", "COM1", "Адрес COM-порта")
	baudRate       = flag.Int("baud", 9600, "Скорость передачи данных (baud rate)")
	maxConnections = flag.Int("max-conn", 5, "Максимальное число одновременных соединений")
)

func main() {
	flag.Parse()

	// Настройка COM-порта
	mode := &serial.Mode{
		BaudRate: *baudRate,
	}
	port, err := serial.Open(*comPort, mode)
	if err != nil {
		log.Fatalf("Не удалось открыть COM-порт: %v", err)
	}
	defer port.Close()

	// Семафор для ограничения числа соединений
	semaphore := make(chan struct{}, *maxConnections)

	// Запуск TCP-сервера
	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Не удалось запустить TCP-сервер: %v", err)
	}
	defer listener.Close()
	log.Printf("Сервер запущен и слушает на %s", *listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при принятии соединения: %v", err)
			continue
		}

		// Проверка числа активных соединений
		select {
		case semaphore <- struct{}{}: // Захват семафора
			log.Printf("Клиент подключен: %s (активных соединений: %d/%d)", conn.RemoteAddr(), len(semaphore), *maxConnections)
			go handleClient(conn, port, semaphore)
		default:
			// Достигнуто максимальное число соединений
			log.Printf("Достигнуто максимальное число соединений. Отклонение подключения от %s", conn.RemoteAddr())
			conn.Close()
		}
	}
}

func handleClient(conn net.Conn, port serial.Port, semaphore chan struct{}) {
	defer func() {
		conn.Close()
		<-semaphore // Освобождение семафора
		log.Printf("Клиент отключен: %s (активных соединений: %d/%d)", conn.RemoteAddr(), len(semaphore), cap(semaphore))
	}()

	// Создаем буферизированный читатель для COM-порта
	reader := bufio.NewReader(port)

	for {
		// Чтение строки с COM-порта
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				log.Println("COM-порт закрыт")
				return
			}
			log.Printf("Ошибка чтения с COM-порта: %v", err)
			return
		}

		//добавляем время к строке
		strToSend := fmt.Sprintf("%s\t%s\n", time.Now().Format("20060102150405"), line)
		// Отправка строки клиенту
		_, err = conn.Write([]byte(strToSend))
		if err != nil {
			log.Printf("Ошибка отправки данных клиенту: %v", err)
			return
		}

		// Логирование отправленной строки
		log.Printf("Отправлена строка клиенту: %s", line)
	}
}

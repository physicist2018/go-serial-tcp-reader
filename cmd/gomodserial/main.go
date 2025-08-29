package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"go.bug.st/serial"
)

var (
	listenAddr     = flag.String("listen", ":8080", "Адрес прослушивания TCP-сервера")
	comPort        = flag.String("com", "COM1", "Адрес COM-порта")
	baudRate       = flag.Int("baud", 9600, "Скорость передачи данных (baud rate)")
	maxConnections = flag.Int("max-conn", 1, "Максимальное число одновременных соединений")
)

type ClientManager struct {
	clients    map[net.Conn]bool
	clientsMux sync.RWMutex
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		clients: make(map[net.Conn]bool),
	}
}

func (cm *ClientManager) AddClient(conn net.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	cm.clients[conn] = true
	log.Printf("Клиент подключен: %s (активных соединений: %d)", conn.RemoteAddr(), len(cm.clients))
}

func (cm *ClientManager) RemoveClient(conn net.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	if _, exists := cm.clients[conn]; exists {
		delete(cm.clients, conn)
		conn.Close()
		log.Printf("Клиент отключен: %s (активных соединений: %d)", conn.RemoteAddr(), len(cm.clients))
	}
}

func (cm *ClientManager) BroadcastData(data string) {
	cm.clientsMux.RLock()
	defer cm.clientsMux.RUnlock()

	if len(cm.clients) == 0 {
		return
	}

	for conn := range cm.clients {
		go func(c net.Conn, d string) {
			_, err := c.Write([]byte(d))
			if err != nil {
				log.Printf("Ошибка отправки данных клиенту %s: %v", c.RemoteAddr(), err)
				cm.RemoveClient(c)
			}
		}(conn, data)
	}
}

func (cm *ClientManager) GetClientCount() int {
	cm.clientsMux.RLock()
	defer cm.clientsMux.RUnlock()
	return len(cm.clients)
}

func main() {
	flag.Parse()

	// Инициализация менеджера клиентов
	clientManager := NewClientManager()

	// Запуск горутины для чтения COM-порта
	go readCOMPort(clientManager)

	// Запуск TCP-сервера
	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Не удалось запустить TCP-сервер: %v", err)
	}
	defer listener.Close()
	log.Printf("Сервер запущен и слушает на %s", *listenAddr)

	// Семафор для ограничения числа соединений
	semaphore := make(chan struct{}, *maxConnections)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при принятии соединения: %v", err)
			continue
		}

		// Проверка числа активных соединений
		select {
		case semaphore <- struct{}{}: // Захват семафора
			go handleClient(conn, clientManager, semaphore)
		default:
			// Достигнуто максимальное число соединений
			log.Printf("Достигнуто максимальное число соединений. Отклонение подключения от %s", conn.RemoteAddr())
			conn.Close()
		}
	}
}

func readCOMPort(clientManager *ClientManager) {
	// Настройка COM-порта
	mode := &serial.Mode{
		BaudRate: *baudRate,
	}

	for {
		port, err := serial.Open(*comPort, mode)
		if err != nil {
			log.Printf("Не удалось открыть COM-порт %s: %v. Повторная попытка через 5 секунд...", *comPort, err)
			time.Sleep(5 * time.Second)
			continue
		}

		log.Printf("COM-порт %s открыт успешно", *comPort)
		defer port.Close()

		// Создаем буферизированный читатель для COM-порта
		reader := bufio.NewReader(port)

		for {
			// Чтение строки с COM-порта
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					log.Println("COM-порт закрыт")
				} else {
					log.Printf("Ошибка чтения с COM-порта: %v", err)
				}
				break
			}

			// Добавляем время к строке
			timestamp := time.Now().Format("20060102150405")
			dataWithTime := fmt.Sprintf("%s\t%s", timestamp, line)

			// Отправляем данные всем подключенным клиентам
			if clientManager.GetClientCount() > 0 {
				clientManager.BroadcastData(dataWithTime)
				log.Printf("Данные отправлены %d клиентам: %s", clientManager.GetClientCount(), line)
			}
		}

		// Если произошла ошибка, ждем перед повторным подключением
		log.Printf("Переподключение к COM-порту через 2 секунды...")
		time.Sleep(2 * time.Second)
	}
}

func handleClient(conn net.Conn, clientManager *ClientManager, semaphore chan struct{}) {
	// Добавляем клиента в менеджер
	clientManager.AddClient(conn)

	// Гарантируем, что семафор будет освобожден при выходе
	defer func() {
		clientManager.RemoveClient(conn)
		<-semaphore // Освобождение семафора
		log.Printf("Семафор освобожден. Доступно слотов: %d/%d",
			cap(semaphore)-len(semaphore), cap(semaphore))
	}()

	// Отправляем приветственное сообщение
	welcomeMsg := fmt.Sprintf("Подключение к COM-порту %s установлено. Ожидание данных...\n", *comPort)
	conn.Write([]byte(welcomeMsg))

	// Читаем данные от клиента (для поддержания соединения)
	buf := make([]byte, 1024)
	for {
		_, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Ошибка чтения от клиента %s: %v", conn.RemoteAddr(), err)
			}
			break
		}
		// Игнорируем входящие данные, просто поддерживаем соединение
	}
}

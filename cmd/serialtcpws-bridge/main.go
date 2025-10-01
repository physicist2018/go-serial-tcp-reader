package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"go.bug.st/serial"
)

var (
	listenAddr     = flag.String("listen", ":8080", "Адрес прослушивания TCP-сервера")
	wsAddr         = flag.String("ws", ":8081", "Адрес прослушивания WebSocket-сервера")
	comPort        = flag.String("com", "COM1", "Адрес COM-порта")
	baudRate       = flag.Int("baud", 9600, "Скорость передачи данных (baud rate)")
	maxConnections = flag.Int("max-conn", 10, "Максимальное число одновременных соединений")
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Разрешаем все origin для упрощения
	},
}

type ClientManager struct {
	tcpClients map[net.Conn]bool
	wsClients  map[*websocket.Conn]bool
	clientsMux sync.RWMutex
}

func NewClientManager() *ClientManager {
	return &ClientManager{
		tcpClients: make(map[net.Conn]bool),
		wsClients:  make(map[*websocket.Conn]bool),
	}
}

func (cm *ClientManager) AddTCPClient(conn net.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	cm.tcpClients[conn] = true
	log.Printf("TCP клиент подключен: %s (активных TCP: %d, WS: %d)",
		conn.RemoteAddr(), len(cm.tcpClients), len(cm.wsClients))
}

func (cm *ClientManager) RemoveTCPClient(conn net.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	if _, exists := cm.tcpClients[conn]; exists {
		delete(cm.tcpClients, conn)
		conn.Close()
		log.Printf("TCP клиент отключен: %s (активных TCP: %d, WS: %d)",
			conn.RemoteAddr(), len(cm.tcpClients), len(cm.wsClients))
	}
}

func (cm *ClientManager) AddWSClient(conn *websocket.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	cm.wsClients[conn] = true
	log.Printf("WebSocket клиент подключен: %s (активных TCP: %d, WS: %d)",
		conn.RemoteAddr(), len(cm.tcpClients), len(cm.wsClients))
}

func (cm *ClientManager) RemoveWSClient(conn *websocket.Conn) {
	cm.clientsMux.Lock()
	defer cm.clientsMux.Unlock()
	if _, exists := cm.wsClients[conn]; exists {
		delete(cm.wsClients, conn)
		conn.Close()
		log.Printf("WebSocket клиент отключен: %s (активных TCP: %d, WS: %d)",
			conn.RemoteAddr(), len(cm.tcpClients), len(cm.wsClients))
	}
}

func (cm *ClientManager) BroadcastData(data string) {
	cm.clientsMux.RLock()
	defer cm.clientsMux.RUnlock()

	totalClients := len(cm.tcpClients) + len(cm.wsClients)
	if totalClients == 0 {
		return
	}

	// Отправка TCP клиентам
	for conn := range cm.tcpClients {
		go func(c net.Conn, d string) {
			_, err := c.Write([]byte(d))
			if err != nil {
				log.Printf("Ошибка отправки данных TCP клиенту %s: %v", c.RemoteAddr(), err)
				cm.RemoveTCPClient(c)
			}
		}(conn, data)
	}

	// Отправка WebSocket клиентам
	for conn := range cm.wsClients {
		go func(c *websocket.Conn, d string) {
			err := c.WriteMessage(websocket.TextMessage, []byte(d))
			if err != nil {
				log.Printf("Ошибка отправки данных WebSocket клиенту %s: %v", c.RemoteAddr(), err)
				cm.RemoveWSClient(c)
			}
		}(conn, data)
	}
}

func (cm *ClientManager) GetClientCount() int {
	cm.clientsMux.RLock()
	defer cm.clientsMux.RUnlock()
	return len(cm.tcpClients) + len(cm.wsClients)
}

func main() {
	flag.Parse()

	// Инициализация менеджера клиентов
	clientManager := NewClientManager()

	// Запуск горутины для чтения COM-порта
	go readCOMPort(clientManager)

	// Запуск TCP-сервера
	go startTCPServer(clientManager)

	// Запуск WebSocket сервера
	go startWebSocketServer(clientManager)

	log.Printf("Сервер запущен:")
	log.Printf("  TCP сервер слушает на %s", *listenAddr)
	log.Printf("  WebSocket сервер слушает на %s", *wsAddr)
	log.Printf("  COM-порт: %s, скорость: %d", *comPort, *baudRate)

	// Бесконечный цикл для поддержания работы main
	select {}
}

func startTCPServer(clientManager *ClientManager) {
	listener, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Не удалось запустить TCP-сервер: %v", err)
	}
	defer listener.Close()

	// Семафор для ограничения числа соединений
	semaphore := make(chan struct{}, *maxConnections)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("Ошибка при принятии TCP соединения: %v", err)
			continue
		}

		// Проверка числа активных соединений
		select {
		case semaphore <- struct{}{}: // Захват семафора
			go handleTCPClient(conn, clientManager, semaphore)
		default:
			// Достигнуто максимальное число соединений
			log.Printf("Достигнуто максимальное число соединений. Отклонение TCP подключения от %s", conn.RemoteAddr())
			conn.Close()
		}
	}
}

func startWebSocketServer(clientManager *ClientManager) {
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		handleWebSocketClient(w, r, clientManager)
	})

	http.HandleFunc("/", serveHTML)

	log.Printf("WebSocket сервер запущен на %s", *wsAddr)
	err := http.ListenAndServe(*wsAddr, nil)
	if err != nil {
		log.Fatalf("Не удалось запустить WebSocket сервер: %v", err)
	}
}

func handleTCPClient(conn net.Conn, clientManager *ClientManager, semaphore chan struct{}) {
	// Добавляем клиента в менеджер
	clientManager.AddTCPClient(conn)

	// Гарантируем, что семафор будет освобожден при выходе
	defer func() {
		clientManager.RemoveTCPClient(conn)
		<-semaphore // Освобождение семафора
		log.Printf("TCP семафор освобожден. Доступно слотов: %d/%d",
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
				log.Printf("Ошибка чтения от TCP клиента %s: %v", conn.RemoteAddr(), err)
			}
			break
		}
		// Игнорируем входящие данные, просто поддерживаем соединение
	}
}

func handleWebSocketClient(w http.ResponseWriter, r *http.Request, clientManager *ClientManager) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Ошибка апгрейда до WebSocket: %v", err)
		return
	}

	// Добавляем клиента в менеджер
	clientManager.AddWSClient(conn)

	defer func() {
		clientManager.RemoveWSClient(conn)
	}()

	// Отправляем приветственное сообщение
	welcomeMsg := fmt.Sprintf("Подключение к COM-порту %s установлено. Ожидание данных...", *comPort)
	conn.WriteMessage(websocket.TextMessage, []byte(welcomeMsg))

	// Обрабатываем сообщения от клиента
	for {
		messageType, p, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Ошибка чтения от WebSocket клиента %s: %v", conn.RemoteAddr(), err)
			break
		}

		// Логируем входящие сообщения (если нужно)
		if messageType == websocket.TextMessage {
			log.Printf("Получено сообщение от WebSocket клиента %s: %s", conn.RemoteAddr(), string(p))
			// Здесь можно обработать команды от клиента, если необходимо
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

func serveHTML(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>COM Port to WebSocket Bridge</title>
    <meta charset="UTF-8">
    <style>
        body {
            font-family: Arial, sans-serif;
            margin: 20px;
            background-color: #f5f5f5;
        }
        .container {
            max-width: 1200px;
            margin: 0 auto;
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
        }
        .status {
            padding: 10px;
            margin: 10px 0;
            border-radius: 4px;
            font-weight: bold;
        }
        .connected { background-color: #d4edda; color: #155724; }
        .disconnected { background-color: #f8d7da; color: #721c24; }
        .stats {
            background-color: #e2e3e5;
            color: #383d41;
            padding: 10px;
            border-radius: 4px;
            margin: 10px 0;
        }
        #output {
            border: 1px solid #ddd;
            padding: 15px;
            height: 500px;
            overflow-y: auto;
            margin-top: 10px;
            background-color: #fafafa;
            font-family: 'Courier New', monospace;
            font-size: 14px;
        }
        .data-line {
            margin: 5px 0;
            padding: 2px 5px;
            border-left: 3px solid transparent;
        }
        .data-line:hover {
            background-color: #f0f0f0;
        }
        .timestamp {
            color: #0066cc;
            font-weight: bold;
        }
        .values {
            color: #009900;
        }
        .info { color: #666; }
        .error { color: #dc3545; }
        button {
            padding: 10px 20px;
            margin: 5px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 16px;
        }
        .btn-connect { background-color: #28a745; color: white; }
        .btn-disconnect { background-color: #dc3545; color: white; }
    </style>
</head>
<body>
    <div class="container">
        <h1>📡 COM Port to WebSocket Bridge</h1>
        <p>Мост для передачи данных с COM-порта через TCP и WebSocket</p>

        <div class="stats" id="stats">
            COM-порт: ` + *comPort + ` | Скорость: ` + fmt.Sprint(*baudRate) + ` бод
        </div>

        <div class="status disconnected" id="status">
            Не подключено
        </div>

        <div>
            <button class="btn-connect" onclick="connect()">Подключиться</button>
            <button class="btn-disconnect" onclick="disconnect()" disabled id="disconnectBtn">Отключиться</button>
        </div>

        <h3>Данные с COM-порта:</h3>
        <div id="output"></div>
    </div>

    <script>
        let ws = null;
        let reconnectAttempts = 0;
        const maxReconnectAttempts = 5;

        function connect() {
            const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            const wsUrl = protocol + '//' + window.location.hostname + ':8081/ws';

            try {
                ws = new WebSocket(wsUrl);

                ws.onopen = function(event) {
                    updateStatus('connected', 'Подключено к WebSocket серверу');
                    document.getElementById('disconnectBtn').disabled = false;
                    reconnectAttempts = 0;
                };

                ws.onmessage = function(event) {
                    parseData(event.data);
                };

                ws.onclose = function(event) {
                    updateStatus('disconnected', 'Отключено от сервера');
                    document.getElementById('disconnectBtn').disabled = true;

                    // Автопереподключение
                    if (reconnectAttempts < maxReconnectAttempts) {
                        reconnectAttempts++;
                        setTimeout(connect, 2000);
                    }
                };

                ws.onerror = function(error) {
                    updateStatus('error', 'Ошибка подключения');
                    logError('WebSocket ошибка: ' + error);
                };

            } catch (error) {
                logError('Ошибка: ' + error.message);
            }
        }

        function disconnect() {
            if (ws) {
                ws.close();
                ws = null;
            }
            reconnectAttempts = maxReconnectAttempts;
        }

        function parseData(data) {
            const lines = data.split('\n');

            lines.forEach(line => {
                if (line.trim() === '') return;

                // Информационные строки
                if (!/^\d/.test(line)) {
                    logInfo(line);
                    return;
                }

                // Строки с данными
                const parts = line.split('\t');
                if (parts.length >= 2) {
                    const timestampStr = parts[0];
                    const dataStr = parts[1];

                    // Проверяем формат timestamp (14 цифр)
                    if (/^\d{14}$/.test(timestampStr)) {
                        parseDataLine(timestampStr, dataStr);
                    } else {
                        logInfo(line);
                    }
                } else {
                    logInfo(line);
                }
            });
        }

        function parseDataLine(timestampStr, dataStr) {
            const datetime = parseTimestamp(timestampStr);

            const pMatch = dataStr.match(/P:([\d.-]+)/);
            const t1Match = dataStr.match(/T1:([\d.-]+)/);
            const depthMatch = dataStr.match(/Depth:([\d.-]+)/);
            const altMatch = dataStr.match(/Alt:([\d.-]+)/);
            const t2Match = dataStr.match(/T2:([\d.-]+)/);

            let resultHtml = '';

            if (pMatch && t1Match && depthMatch && altMatch && t2Match) {
                resultHtml = '<div class="data-line">' +
                    '<span class="timestamp">' + timestampStr + '</span> - ' +
                    '<span class="values">' +
                    'DateTime: ' + datetime + ', ' +
                    'P: ' + pMatch[1] + ', ' +
                    'T1: ' + t1Match[1] + ', ' +
                    'Depth: ' + depthMatch[1] + ', ' +
                    'Alt: ' + altMatch[1] + ', ' +
                    'T2: ' + t2Match[1] +
                    '</span>' +
                    '</div>';
            } else {
                resultHtml = '<div class="data-line">' + timestampStr + '\t' + dataStr + '</div>';
            }

            logHTML(resultHtml);
        }

        function parseTimestamp(timestampStr) {
            const year = timestampStr.substr(0, 4);
            const month = timestampStr.substr(4, 2);
            const day = timestampStr.substr(6, 2);
            const hour = timestampStr.substr(8, 2);
            const minute = timestampStr.substr(10, 2);
            const second = timestampStr.substr(12, 2);

            return year + '-' + month + '-' + day + ' ' + hour + ':' + minute + ':' + second;
        }

        function updateStatus(type, message) {
            const status = document.getElementById('status');
            status.className = 'status ' + type;
            status.textContent = message;
        }

        function logInfo(message) {
            const output = document.getElementById('output');
            output.innerHTML += '<div class="data-line info">' + message + '</div>';
            output.scrollTop = output.scrollHeight;
        }

        function logError(message) {
            const output = document.getElementById('output');
            output.innerHTML += '<div class="data-line error">' + message + '</div>';
            output.scrollTop = output.scrollHeight;
        }

        function logHTML(html) {
            const output = document.getElementById('output');
            output.innerHTML += html;
            output.scrollTop = output.scrollHeight;
        }

        // Автоподключение при загрузке страницы
        window.onload = connect;
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

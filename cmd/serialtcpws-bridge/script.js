let ws = null;
let reconnectAttempts = 0;
const maxReconnectAttempts = 5;

function connect() {
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = protocol + "//" + window.location.hostname + ":8081/ws";

  try {
    ws = new WebSocket(wsUrl);

    ws.onopen = function (event) {
      updateStatus("connected", "Подключено к WebSocket серверу");
      document.getElementById("disconnectBtn").disabled = false;
      reconnectAttempts = 0;
    };

    ws.onmessage = function (event) {
      parseData(event.data);
    };

    ws.onclose = function (event) {
      updateStatus("disconnected", "Отключено от сервера");
      document.getElementById("disconnectBtn").disabled = true;

      // Автопереподключение
      if (reconnectAttempts < maxReconnectAttempts) {
        reconnectAttempts++;
        setTimeout(connect, 2000);
      }
    };

    ws.onerror = function (error) {
      updateStatus("error", "Ошибка подключения");
      logError("WebSocket ошибка: " + error);
    };
  } catch (error) {
    logError("Ошибка: " + error.message);
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
  const lines = data.split("\n");

  lines.forEach((line) => {
    if (line.trim() === "") return;

    // Информационные строки
    if (!/^\d/.test(line)) {
      logInfo(line);
      return;
    }

    // Строки с данными
    const parts = line.split("\t");
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

  let resultHtml = "";

  if (pMatch && t1Match && depthMatch && altMatch && t2Match) {
    resultHtml = `
            <div class="data-line">
                <span class="timestamp">${timestampStr}</span> -
                <span class="values">
                    DateTime: ${datetime},
                    P: ${pMatch[1]},
                    T1: ${t1Match[1]},
                    Depth: ${depthMatch[1]},
                    Alt: ${altMatch[1]},
                    T2: ${t2Match[1]}
                </span>
            </div>
        `;
  } else {
    resultHtml = `<div class="data-line">${timestampStr}\t${dataStr}</div>`;
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

  return `${year}-${month}-${day} ${hour}:${minute}:${second}`;
}

function updateStatus(type, message) {
  const status = document.getElementById("status");
  status.className = "status " + type;
  status.textContent = message;
}

function logInfo(message) {
  const output = document.getElementById("output");
  output.innerHTML += `<div class="data-line info">${message}</div>`;
  output.scrollTop = output.scrollHeight;
}

function logError(message) {
  const output = document.getElementById("output");
  output.innerHTML += `<div class="data-line error">${message}</div>`;
  output.scrollTop = output.scrollHeight;
}

function logHTML(html) {
  const output = document.getElementById("output");
  output.innerHTML += html;
  output.scrollTop = output.scrollHeight;
}

// Автоподключение при загрузке страницы
window.onload = connect;

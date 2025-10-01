/*
 * Читает данные с датчиков MS5837 и TSYS01
 * если не удалось инициализировать датчики, выводит ошибку и заускает бесконечный цикл
 * если программа зависает, она будет перезагружена спустя 8 секунд по сторожевому таймеру
 */
#include <Wire.h>
#include "TSYS01.h"
#include "MS5837.h"
#include <avr/wdt.h> // Подключаем библиотеку для работы с WDT

MS5837 sensor_ms5837;
TSYS01 sensor_tsys01;
float P = 0, T1 = 0, T2 = 0, Depth = 0, Alt = 0;

int ms5837_ok = false;
int tsys01_ok = false;

void setup() {
  wdt_enable(WDTO_8S); // Включаем сторожевой таймер с интервалом примерно 8 секунд

  Serial.begin(9600);
  Serial.println("Starting");
  
  Wire.begin();
  #define F_CPU 16000000UL
  int desiredFrequency = 50000; //50kHz
  // Calculate TWBR value for the desired frequency
  // TWBR = ((F_CPU / desiredFrequency) - 16) / 2;
  TWBR = ((F_CPU / desiredFrequency) - 16) / 2;
  
  int nretry = 0;

  // ищем MS5837
  ms5837_ok = sensor_ms5837.init();
  while (!ms5837_ok && (nretry < 3)) {
    Serial.println("MS5837 Init failed, retry in 1 sec");
    nretry++;
    delay(1000);
    ms5837_ok = sensor_ms5837.init();
  }

  sensor_ms5837.setModel(MS5837::MS5837_30BA);
  sensor_ms5837.setFluidDensity(1029);

  nretry = 0;

  // ищем TSYS01
  tsys01_ok = sensor_tsys01.init();
  while (!tsys01_ok && (nretry < 3)) {
    Serial.println("TSYS01 Init failed, retry in 1 sec");
    nretry++;
    delay(1000);
    tsys01_ok = sensor_tsys01.init();
  }

  if (!tsys01_ok || !ms5837_ok) {
    Serial.println("It is nesessary for both sensors to work together");
    while (1) {
      delay(1000);
    }
  }
}

void loop() {
  wdt_reset(); // Обнуляем сторожевой таймер каждую итерацию цикла
  
  
  sensor_ms5837.read();
  delay(100);
  
  P = sensor_ms5837.pressure();
  T1 = sensor_ms5837.temperature();
  Depth = sensor_ms5837.depth();
  Alt = sensor_ms5837.altitude();
  sensor_tsys01.read();
  T2 = sensor_tsys01.temperature();

  if ((P>500) && (T1>-3) && (T2>-3) && (P<6000) && (T1<100) && (T2<100)){
      Serial.print("P:");
      Serial.print(P);
      Serial.print(", T1:");
      Serial.print(T1);
      Serial.print(", Depth:");
      Serial.print(Depth);
      Serial.print(", Alt:");
      Serial.print(Alt);
      Serial.print(", ");
      Serial.print("T2:");
      Serial.println(T2);
  }
 

  delay(900);
}


// #include <Wire.h>
// #include "TSYS01.h"
// #include "MS5837.h"

// MS5837 sensor1;
// TSYS01 sensor2;

// int ms5837_ok = false;
// int tsys01_ok = false;

// void setup() {
//   // put your setup code here, to run once:
//   Serial.begin(9600);
//   Serial.println("Starting");
//   Wire.begin();
//   int nretry = 0;

//   ms5837_ok = sensor1.init();
//   while(!ms5837_ok & (nretry<3)){
//     Serial.println("MS5837 Init failed, retry in 2 sec");
//     nretry+=1;
//     delay(2000);
//     ms5837_ok = sensor1.init();
//   }

//   sensor1.setModel(MS5837::MS5837_30BA);
//   sensor1.setFluidDensity(1029);

//   nretry = 0;
//   tsys01_ok = sensor2.init();
//   while(!tsys01_ok & (nretry<3)){
//     Serial.println("TSYS01 Init failed, retry in 2 sec");
//     nretry+=1;
//     delay(2000);
//     tsys01_ok = sensor2.init();
//   }

// }

// void loop() {
//   // put your main code here, to run repeatedly:
//   if(ms5837_ok){
//     sensor1.read();
//     Serial.print("P:");
//     Serial.print(sensor1.pressure());
//     Serial.print(", T1:");
//     Serial.print(sensor1.temperature());
//     Serial.print(", Depth:");
//     Serial.print(sensor1.depth());
//     Serial.print(", Alt:");
//     Serial.print(sensor1.altitude());
//     Serial.print(", ");

//   }else{
//     Serial.println("P:-9999.99, T1:-9999.99, Depth:-9999.99, Alt:-9999.99, ");
//   }
//   if(tsys01_ok){
//     sensor2.read();
//     Serial.print("T2:");
//     Serial.print(sensor2.temperature());
//     Serial.println();
//   }else{
//     Serial.println("T2:-9999.99");
//   }
//   delay(1000);
// }

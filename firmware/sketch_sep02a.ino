#include <Wire.h>
#include "TSYS01.h"
#include "MS5837.h"

MS5837 sensor1;
TSYS01 sensor2;

int ms5837_ok = false;
int tsys01_ok = false;

void setup() {
  // put your setup code here, to run once:
  Serial.begin(9600);
  Serial.println("Starting");
  Wire.begin();
  int nretry = 0;

  ms5837_ok = sensor1.init();
  while(!ms5837_ok & (nretry<3)){
    Serial.println("MS5837 Init failed, retry in 2 sec");
    nretry+=1;
    delay(2000);
    ms5837_ok = sensor1.init();
  }
  
  sensor1.setModel(MS5837::MS5837_30BA);
  sensor1.setFluidDensity(1029);

  nretry = 0;
  tsys01_ok = sensor2.init();
  while(!tsys01_ok & (nretry<3)){
    Serial.println("TSYS01 Init failed, retry in 2 sec");
    nretry+=1;
    delay(2000);
    tsys01_ok = sensor2.init();
  }
  
}

void loop() {
  // put your main code here, to run repeatedly:
  if(ms5837_ok){
    sensor1.read();
    Serial.print("P:");
    Serial.print(sensor1.pressure());
    Serial.print(", T1:");
    Serial.print(sensor1.temperature());
    Serial.print(", Depth:");
    Serial.print(sensor1.depth());
    Serial.print(", Alt:");
    Serial.print(sensor1.altitude());
    Serial.print(", ");
    
  }else{
    Serial.println("P:-9999.99, T1:-9999.99, Depth:-9999.99, Alt:-9999.99, ");
  }
  if(tsys01_ok){
    sensor2.read();
    Serial.print("T2:");
    Serial.print(sensor2.temperature());
    Serial.println();
  }else{
    Serial.println("T2:-9999.99");
  }
  delay(1000);
}

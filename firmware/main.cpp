#include <Wire.h>

// MS5837 I2C address (CSB pin grounded -> 0x76)
#define MS5837_ADDR 0x76
// TSYS01 I2C address (fixed at 0x77)
#define TSYS01_ADDR 0x77

// MS5837 commands
#define MS5837_RESET 0x1E
#define MS5837_ADC_READ 0x00
#define MS5837_PROM_READ 0xA0
#define MS5837_CONVERT_D1_256 0x40
#define MS5837_CONVERT_D2_256 0x50

// TSYS01 commands
#define TSYS01_RESET 0xFE
#define TSYS01_ADC_READ 0x00
#define TSYS01_CONVERT 0x48
#define TSYS01_PROM_READ 0xA0

// Calibration data storage
uint16_t ms5837_c[8];
uint16_t tsys01_c[8];

bool ms5837_initialized = false;
bool tsys01_initialized = false;

void setup() {
  Serial.begin(9600);
  Wire.begin();
  
  // Initialize sensors with error checking
  if (!initializeMS5837()) {
    Serial.println("Error: MS5837 initialization failed!");
    while(1); // Stop if sensor not found
  }
  
  if (!initializeTSYS01()) {
    Serial.println("Error: TSYS01 initialization failed!");
    while(1); // Stop if sensor not found
  }
  
  Serial.println("All sensors initialized successfully");
  delay(1000);
}

void loop() {
  // Read and display MS5837 data
  float temperature_ms5837, pressure;
  if (!readMS5837(&temperature_ms5837, &pressure)) {
    Serial.println("Error reading MS5837!");
    delay(1000);
    return;
  }
  
  // Read and display TSYS01 data
  float temperature_tsys01;
  if (!readTSYS01(&temperature_tsys01)) {
    Serial.println("Error reading TSYS01!");
    delay(1000);
    return;
  }
  
  // Print results in CSV format for easier parsing
  Serial.print("MS5837_Temp:");
  Serial.print(temperature_ms5837, 2);
  Serial.print(",Pressure:");
  Serial.print(pressure, 2);
  Serial.print(",TSYS01_Temp:");
  Serial.print(temperature_tsys01, 2);
  Serial.println();
  
  delay(1000);
}

bool initializeMS5837() {
  // Reset MS5837
  Wire.beginTransmission(MS5837_ADDR);
  Wire.write(MS5837_RESET);
  if (Wire.endTransmission() != 0) {
    Serial.println("MS5837 not found on I2C bus");
    return false;
  }
  delay(10);
  
  // Read calibration data
  for (uint8_t i = 0; i < 7; i++) {
    Wire.beginTransmission(MS5837_ADDR);
    Wire.write(MS5837_PROM_READ + i * 2);
    if (Wire.endTransmission() != 0) {
      Serial.print("Error accessing MS5837 PROM register: ");
      Serial.println(i);
      return false;
    }
    
    if (Wire.requestFrom(MS5837_ADDR, 2) != 2) {
      Serial.print("Error reading MS5837 PROM data: ");
      Serial.println(i);
      return false;
    }
    
    ms5837_c[i] = (Wire.read() << 8) | Wire.read();
  }
  
  // Simple CRC check (optional but recommended)
  uint16_t crc = (ms5837_c[0] >> 12) & 0x000F;
  uint16_t calculated_crc = 0;
  for (uint8_t i = 0; i < 6; i++) {
    calculated_crc ^= (ms5837_c[i] & 0x00FF);
    calculated_crc ^= (ms5837_c[i] >> 8);
  }
  calculated_crc &= 0x000F;
  
  if (crc != calculated_crc) {
    Serial.println("MS5837 CRC mismatch! Sensor may be faulty");
    // Continue anyway but warn user
  }
  
  ms5837_initialized = true;
  return true;
}

bool initializeTSYS01() {
  // Reset TSYS01
  Wire.beginTransmission(TSYS01_ADDR);
  Wire.write(TSYS01_RESET);
  if (Wire.endTransmission() != 0) {
    Serial.println("TSYS01 not found on I2C bus");
    return false;
  }
  delay(10);
  
  // Read calibration data
  for (uint8_t i = 0; i < 8; i++) {
    Wire.beginTransmission(TSYS01_ADDR);
    Wire.write(TSYS01_PROM_READ + i * 2);
    if (Wire.endTransmission() != 0) {
      Serial.print("Error accessing TSYS01 PROM register: ");
      Serial.println(i);
      return false;
    }
    
    if (Wire.requestFrom(TSYS01_ADDR, 2) != 2) {
      Serial.print("Error reading TSYS01 PROM data: ");
      Serial.println(i);
      return false;
    }
    
    tsys01_c[i] = (Wire.read() << 8) | Wire.read();
  }
  
  tsys01_initialized = true;
  return true;
}

bool readMS5837(float *temperature, float *pressure) {
  if (!ms5837_initialized) return false;
  
  // Convert D1 (pressure)
  Wire.beginTransmission(MS5837_ADDR);
  Wire.write(MS5837_CONVERT_D1_256);
  if (Wire.endTransmission() != 0) return false;
  delay(1);
  
  uint32_t D1 = readADC(MS5837_ADDR);
  if (D1 == 0) return false;
  
  // Convert D2 (temperature)
  Wire.beginTransmission(MS5837_ADDR);
  Wire.write(MS5837_CONVERT_D2_256);
  if (Wire.endTransmission() != 0) return false;
  delay(1);
  
  uint32_t D2 = readADC(MS5837_ADDR);
  if (D2 == 0) return false;
  
  // Calculate temperature and pressure (first order)
  int32_t dT = D2 - (uint32_t)ms5837_c[5] * 256;
  int64_t OFF = (int64_t)ms5837_c[2] * 131072 + (int64_t)ms5837_c[4] * dT / 64;
  int64_t SENS = (int64_t)ms5837_c[1] * 65536 + (int64_t)ms5837_c[3] * dT / 128;
  
  *temperature = 2000 + dT * ms5837_c[6] / 8388608.0;
  
  // Second order temperature compensation
  if (*temperature < 20.0) {
    int64_t T2 = pow(dT, 2) / 2147483648;
    int64_t OFF2 = 61 * pow((*temperature - 2000.0), 2) / 16;
    int64_t SENS2 = 29 * pow((*temperature - 2000.0), 2) / 16;
    
    if (*temperature < -15.0) {
      OFF2 = OFF2 + 20 * pow((*temperature + 1500.0), 2);
      SENS2 = SENS2 + 12 * pow((*temperature + 1500.0), 2);
    }
    
    *temperature -= T2;
    OFF -= OFF2;
    SENS -= SENS2;
  }
  
  *pressure = (D1 * SENS / 2097152 - OFF) / 32768.0 / 100.0;
  
  return true;
}

bool readTSYS01(float *temperature) {
  if (!tsys01_initialized) return false;
  
  // Start conversion
  Wire.beginTransmission(TSYS01_ADDR);
  Wire.write(TSYS01_CONVERT);
  if (Wire.endTransmission() != 0) return false;
  delay(10);
  
  // Read ADC
  uint32_t adc = readADC(TSYS01_ADDR);
  if (adc == 0) return false;
  
  // Calculate temperature using proper calibration from datasheet
  int64_t temp = (int64_t)tsys01_c[1] * (int64_t)adc;
  temp -= (int64_t)tsys01_c[2] * 256000;
  temp *= 100;
  temp /= (int64_t)tsys01_c[3] * (int64_t)adc;
  temp -= (int64_t)tsys01_c[4] * 1000;
  
  *temperature = temp / 100.0;
  
  return true;
}

uint32_t readADC(uint8_t address) {
  Wire.beginTransmission(address);
  Wire.write(MS5837_ADC_READ);
  if (Wire.endTransmission() != 0) {
    return 0;
  }
  
  if (Wire.requestFrom(address, 3) != 3) {
    return 0;
  }
  
  uint32_t adc = Wire.read() << 16;
  adc |= Wire.read() << 8;
  adc |= Wire.read();
  
  return adc;
}



#!/bin/bash

echo "Upload firmware via USBAsp"
avrdude -p atmega328p -c usbasp -P usb -B 1 -U flash:w:$1:i

if [ $? -eq 0 ];then
 echo "Upload successfull"
else
 echo "Error"
 exit 1
fi


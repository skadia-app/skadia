package fitimport

import (
	"fmt"
	"strings"

	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/skadia-app/skadia/services/api/internal/model"
)

func ExtractSensors(deviceInfos []*mesgdef.DeviceInfo) []model.SensorRecord {
	type sensorKey struct {
		serial    uint32
		devIndex  uint8
		hasSerial bool
	}

	seenBySerial := make(map[uint32]int) // serial → index in result
	seenByIndex := make(map[uint8]int)   // deviceIndex → index in result
	var result []model.SensorRecord

	for _, di := range deviceInfos {
		if di.SourceType != typedef.SourceTypeAntplus && di.SourceType != typedef.SourceTypeBluetooth {
			continue
		}

		// Use dynamic field interpretation for device type
		_, dtVal := di.GetDeviceType()
		deviceTypeStr := strings.ToLower(fmt.Sprintf("%v", dtVal))
		if deviceTypeStr == "" || strings.Contains(deviceTypeStr, "(") {
			continue
		}

		sourceType := "antplus"
		if di.SourceType == typedef.SourceTypeBluetooth {
			sourceType = "bluetooth"
		}

		serial := di.SerialNumber
		hasValidSerial := serial != 0 && serial != 0xFFFFFFFF

		// Try to find existing sensor entry
		existingIdx := -1
		if hasValidSerial {
			if idx, ok := seenBySerial[serial]; ok {
				existingIdx = idx
			}
		}
		if existingIdx == -1 {
			if idx, ok := seenByIndex[uint8(di.DeviceIndex)]; ok {
				existingIdx = idx
			}
		}

		// ANT+ device number (uint16z, invalid = 0)
		hasValidAntNum := di.AntDeviceNumber != basetype.Uint16zInvalid
		antNum := int32(di.AntDeviceNumber)

		if existingIdx >= 0 {
			// Merge: add device type to existing entry
			existing := &result[existingIdx]
			if !containsStr(existing.AllDeviceTypes, deviceTypeStr) {
				existing.AllDeviceTypes = append(existing.AllDeviceTypes, deviceTypeStr)
			}
			// Update fields if they were missing
			if hasValidSerial && existing.SerialNumber == nil {
				s := int64(serial)
				existing.SerialNumber = &s
			}
			if hasValidAntNum && existing.AntDeviceNumber == nil {
				existing.AntDeviceNumber = &antNum
			}
			if existing.BatteryStatus == "" && di.BatteryStatus != typedef.BatteryStatusInvalid {
				existing.BatteryStatus = strings.ToLower(di.BatteryStatus.String())
			}
			continue
		}

		// New sensor entry
		rec := model.SensorRecord{
			DeviceIndex:       int16(di.DeviceIndex),
			SourceType:        sourceType,
			AntplusDeviceType: deviceTypeStr,
			AllDeviceTypes:    []string{deviceTypeStr},
			ManufacturerID:    int16(di.Manufacturer),
			SoftwareVersion:   float64(di.SoftwareVersion) / 100.0,
			HardwareVersion:   int16(di.HardwareVersion),
		}

		if hasValidSerial {
			s := int64(serial)
			rec.SerialNumber = &s
		}

		if hasValidAntNum {
			rec.AntDeviceNumber = &antNum
		}

		mfr := di.Manufacturer.String()
		if mfr != "" && !strings.HasPrefix(mfr, "Manufacturer(") {
			rec.Manufacturer = mfr
		}

		// Use dynamic field interpretation for product
		prodName, prodVal := di.GetProduct()
		if prodName == "garmin_product" {
			gp := prodVal.(typedef.GarminProduct)
			if gp != typedef.GarminProductInvalid {
				rec.GarminProduct = gp.String()
				rec.GarminProductID = int32(gp)
			}
		}

		if di.ProductName != "" {
			rec.ProductName = di.ProductName
		}

		if di.BatteryStatus != typedef.BatteryStatusInvalid {
			rec.BatteryStatus = strings.ToLower(di.BatteryStatus.String())
		}
		if di.BatteryVoltage != 0xFFFF && di.BatteryVoltage != 0 {
			rec.BatteryVoltage = float64(di.BatteryVoltage) / 256.0
		}

		idx := len(result)
		result = append(result, rec)

		if hasValidSerial {
			seenBySerial[serial] = idx
		}
		seenByIndex[uint8(di.DeviceIndex)] = idx
	}

	return result
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

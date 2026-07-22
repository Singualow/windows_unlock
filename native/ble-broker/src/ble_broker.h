#pragma once

#include <cstdint>

#ifdef PROXIMITY_BLE_BROKER_EXPORTS
#define PU_BLE_API extern "C" __declspec(dllexport)
#else
#define PU_BLE_API extern "C" __declspec(dllimport)
#endif

struct PU_BLE_FRAME {
    std::uint64_t address;
    std::int16_t rssi;
    std::uint16_t company_id;
    std::uint8_t length;
    std::uint8_t data[64];
};

PU_BLE_API int __stdcall PU_BLE_Start();
PU_BLE_API int __stdcall PU_BLE_Next(PU_BLE_FRAME* frame, std::uint32_t timeout_ms);
PU_BLE_API std::uint64_t __stdcall PU_BLE_Received();
PU_BLE_API int __stdcall PU_BLE_Status();
PU_BLE_API int __stdcall PU_BLE_Error();
PU_BLE_API void __stdcall PU_BLE_Stop();

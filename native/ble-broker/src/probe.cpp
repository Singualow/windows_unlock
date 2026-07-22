#include "ble_broker.h"

#include <windows.h>
#include <winrt/Windows.Devices.Bluetooth.h>
#include <winrt/Windows.Devices.Radios.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/base.h>

#include <chrono>
#include <cstdio>
#include <set>

int wmain() {
	std::setvbuf(stdout, nullptr, _IONBF, 0);
	try {
		winrt::init_apartment(winrt::apartment_type::multi_threaded);
		{
			auto adapter = winrt::Windows::Devices::Bluetooth::BluetoothAdapter::GetDefaultAsync().get();
			if (!adapter) {
				std::fprintf(stderr, "default Bluetooth adapter: unavailable\n");
				return 1;
			}
			auto radio = adapter.GetRadioAsync().get();
			std::printf("adapter: central_role=%s peripheral_role=%s radio_state=%d\n",
			            adapter.IsCentralRoleSupported() ? "true" : "false",
			            adapter.IsPeripheralRoleSupported() ? "true" : "false",
			            static_cast<int>(radio.State()));
		}
		winrt::uninit_apartment();
	} catch (winrt::hresult_error const& error) {
		std::fprintf(stderr, "adapter query error: 0x%08x\n", static_cast<unsigned>(error.code().value));
	}
    const int start = PU_BLE_Start();
    if (start != ERROR_SUCCESS) {
        std::fprintf(stderr, "broker start error: %d\n", start);
        return 1;
    }
    unsigned long manufacturer_frames = 0;
	unsigned long target_frames = 0;
	std::set<std::uint16_t> companies;
    const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(20);
    while (std::chrono::steady_clock::now() < deadline) {
        PU_BLE_FRAME frame{};
        const int result = PU_BLE_Next(&frame, 1000);
        if (result < 0) {
            std::fprintf(stderr, "broker read error: %d\n", result);
            PU_BLE_Stop();
            return 1;
        }
        if (result == 1) {
			++manufacturer_frames;
			if (companies.insert(frame.company_id).second) {
				std::printf("manufacturer: company=0x%04x length=%u rssi=%d\n",
				            frame.company_id, frame.length, frame.rssi);
			}
			if (frame.company_id == 0xffff && frame.length == 13) ++target_frames;
        }
    }
	const auto received = PU_BLE_Received();
	const auto status = PU_BLE_Status();
	const auto error = PU_BLE_Error();
    PU_BLE_Stop();
	std::printf("summary: received_events=%llu manufacturer_frames=%lu target_frames=%lu watcher_status=%d watcher_error=%d\n",
	            static_cast<unsigned long long>(received), manufacturer_frames, target_frames, status, error);
    return 0;
}

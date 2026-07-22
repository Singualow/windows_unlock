#define PROXIMITY_BLE_BROKER_EXPORTS
#include "ble_broker.h"

#include <windows.h>
#include <winrt/Windows.Devices.Bluetooth.Advertisement.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Storage.Streams.h>
#include <winrt/base.h>

#include <algorithm>
#include <atomic>
#include <condition_variable>
#include <deque>
#include <memory>
#include <mutex>
#include <thread>
#include <vector>

namespace {

using namespace winrt;
using namespace Windows::Devices::Bluetooth::Advertisement;
using namespace Windows::Storage::Streams;

constexpr std::uint16_t kPersonalCompanyId = 0xffff;
constexpr std::size_t kMaximumQueuedFrames = 128;

class Broker final {
public:
    int Start() {
        std::unique_lock lock(mutex_);
        if (worker_.joinable()) return ERROR_ALREADY_INITIALIZED;
        stopping_ = false;
        ready_ = false;
        startup_error_ = ERROR_SUCCESS;
        worker_ = std::thread([this] { Run(); });
        ready_cv_.wait(lock, [this] { return ready_; });
        return startup_error_;
    }

    int Next(PU_BLE_FRAME* frame, std::uint32_t timeout_ms) {
        if (!frame) return -static_cast<int>(ERROR_INVALID_PARAMETER);
        std::unique_lock lock(mutex_);
        if (!frames_cv_.wait_for(lock, std::chrono::milliseconds(timeout_ms), [this] {
                return !frames_.empty() || stopping_;
            })) {
            return 0;
        }
        if (frames_.empty()) return 0;
        *frame = frames_.front();
        frames_.pop_front();
        return 1;
    }

    void Stop() {
        {
            std::lock_guard lock(mutex_);
            stopping_ = true;
        }
        stop_cv_.notify_all();
        frames_cv_.notify_all();
        if (worker_.joinable()) worker_.join();
    }

    ~Broker() { Stop(); }

	std::uint64_t Received() const noexcept { return received_.load(); }
	int Status() const noexcept { return status_.load(); }
	int Error() const noexcept { return error_.load(); }

private:
    void Run() noexcept {
        try {
            init_apartment(apartment_type::multi_threaded);
            {
                BluetoothLEAdvertisementWatcher watcher;
                watcher.ScanningMode(BluetoothLEScanningMode::Active);
                const auto received = watcher.Received([this](auto const&, BluetoothLEAdvertisementReceivedEventArgs const& args) {
                    OnAdvertisement(args);
                });
				const auto stopped = watcher.Stopped([this](auto const&, BluetoothLEAdvertisementWatcherStoppedEventArgs const& args) {
					error_.store(static_cast<int>(args.Error()));
					status_.store(static_cast<int>(BluetoothLEAdvertisementWatcherStatus::Aborted));
				});
                watcher.Start();
				status_.store(static_cast<int>(watcher.Status()));
                SignalReady(ERROR_SUCCESS);
                {
					std::unique_lock lock(mutex_);
					while (!stopping_) {
						stop_cv_.wait_for(lock, std::chrono::milliseconds(250));
						status_.store(static_cast<int>(watcher.Status()));
					}
                }
                watcher.Stop();
                for (int attempt = 0; attempt < 40 &&
                     watcher.Status() != BluetoothLEAdvertisementWatcherStatus::Stopped; ++attempt) {
                    std::this_thread::sleep_for(std::chrono::milliseconds(50));
                }
                watcher.Received(received);
				watcher.Stopped(stopped);
				status_.store(static_cast<int>(watcher.Status()));
            }
            uninit_apartment();
        } catch (hresult_error const& error) {
            SignalReady(static_cast<int>(error.code().value));
        } catch (...) {
            SignalReady(static_cast<int>(ERROR_GEN_FAILURE));
        }
    }

    void SignalReady(int error) {
        {
            std::lock_guard lock(mutex_);
            if (ready_) return;
            startup_error_ = error;
            ready_ = true;
            if (error != ERROR_SUCCESS) stopping_ = true;
        }
        ready_cv_.notify_all();
        frames_cv_.notify_all();
    }

    void OnAdvertisement(BluetoothLEAdvertisementReceivedEventArgs const& args) noexcept {
        try {
			received_.fetch_add(1);
			for (auto const& item : args.Advertisement().ManufacturerData()) {
				Enqueue(args, item.CompanyId(), ReadBuffer(item.Data()), 0);
			}
			// Data type 0x16 is Service Data - 16-bit UUID. Supporting it here
			// keeps pairing compatible with the first Android build as well.
			for (auto const& section : args.Advertisement().DataSections()) {
				if (section.DataType() != 0x16) continue;
				auto bytes = ReadBuffer(section.Data());
				if (bytes.size() >= 2 && bytes[0] == 0xf0 && bytes[1] == 0xff) {
					Enqueue(args, kPersonalCompanyId, bytes, 2);
				}
            }
        } catch (...) {
            // A malformed advertisement is ignored without stopping the watcher.
        }
    }

	static std::vector<std::uint8_t> ReadBuffer(IBuffer const& buffer) {
		const std::uint32_t length = buffer.Length();
		std::vector<std::uint8_t> bytes(length);
		if (length != 0) {
			DataReader reader = DataReader::FromBuffer(buffer);
			reader.ReadBytes(bytes);
		}
		return bytes;
	}

	void Enqueue(BluetoothLEAdvertisementReceivedEventArgs const& args, std::uint16_t company,
	             std::vector<std::uint8_t> const& bytes, std::size_t skip) {
		if (bytes.size() < skip) return;
		const std::size_t length = std::min<std::size_t>(bytes.size() - skip, 64);
		PU_BLE_FRAME frame{};
		frame.address = args.BluetoothAddress();
		frame.rssi = args.RawSignalStrengthInDBm();
		frame.company_id = company;
		frame.length = static_cast<std::uint8_t>(length);
		std::copy_n(bytes.data() + skip, length, frame.data);
		{
			std::lock_guard lock(mutex_);
			if (frames_.size() == kMaximumQueuedFrames) frames_.pop_front();
			frames_.push_back(frame);
		}
		frames_cv_.notify_one();
	}

    std::mutex mutex_;
    std::condition_variable ready_cv_;
    std::condition_variable frames_cv_;
    std::condition_variable stop_cv_;
    std::deque<PU_BLE_FRAME> frames_;
    std::thread worker_;
    bool stopping_ = false;
    bool ready_ = false;
    int startup_error_ = ERROR_SUCCESS;
	std::atomic<std::uint64_t> received_{0};
	std::atomic<int> status_{static_cast<int>(BluetoothLEAdvertisementWatcherStatus::Created)};
	std::atomic<int> error_{0};
};

std::mutex g_mutex;
std::unique_ptr<Broker> g_broker;

}  // namespace

int __stdcall PU_BLE_Start() {
    std::lock_guard lock(g_mutex);
    if (g_broker) return ERROR_ALREADY_INITIALIZED;
    auto broker = std::make_unique<Broker>();
    const int result = broker->Start();
    if (result != ERROR_SUCCESS) return result;
    g_broker = std::move(broker);
    return ERROR_SUCCESS;
}

int __stdcall PU_BLE_Next(PU_BLE_FRAME* frame, std::uint32_t timeout_ms) {
    std::lock_guard lock(g_mutex);
    if (!g_broker) return -static_cast<int>(ERROR_INVALID_STATE);
    return g_broker->Next(frame, timeout_ms);
}

std::uint64_t __stdcall PU_BLE_Received() {
	std::lock_guard lock(g_mutex);
	return g_broker ? g_broker->Received() : 0;
}

int __stdcall PU_BLE_Status() {
	std::lock_guard lock(g_mutex);
	return g_broker ? g_broker->Status() : -1;
}

int __stdcall PU_BLE_Error() {
	std::lock_guard lock(g_mutex);
	return g_broker ? g_broker->Error() : -1;
}

void __stdcall PU_BLE_Stop() {
    std::unique_ptr<Broker> broker;
    {
        std::lock_guard lock(g_mutex);
        broker = std::move(g_broker);
    }
    if (broker) broker->Stop();
}

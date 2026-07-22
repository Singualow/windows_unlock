#include <windows.h>
#include <credentialprovider.h>
#include <objbase.h>

#include <iostream>
#include <string>
#include <vector>

namespace {

constexpr wchar_t kProvidersKey[] =
    L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Authentication\\Credential Providers";
constexpr wchar_t kFilterClsid[] = L"{DDC0EED2-ADBE-40b6-A217-EDE16A79A0DE}";
constexpr wchar_t kProximityClsid[] = L"{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}";

std::wstring GuidText(REFGUID guid) {
    wchar_t value[64]{};
    StringFromGUID2(guid, value, ARRAYSIZE(value));
    return value;
}

std::vector<CLSID> RegisteredProviders() {
    HKEY key = nullptr;
    std::vector<CLSID> result;
    if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, kProvidersKey, 0, KEY_READ, &key) != ERROR_SUCCESS) {
        return result;
    }
    for (DWORD index = 0;; ++index) {
        wchar_t name[128]{};
        DWORD chars = ARRAYSIZE(name);
        const LONG status = RegEnumKeyExW(key, index, name, &chars, nullptr, nullptr, nullptr, nullptr);
        if (status == ERROR_NO_MORE_ITEMS) break;
        if (status != ERROR_SUCCESS) continue;
        CLSID clsid{};
        if (SUCCEEDED(CLSIDFromString(name, &clsid))) result.push_back(clsid);
    }
    RegCloseKey(key);
    return result;
}

void ProbeFilter(ICredentialProviderFilter* filter, CREDENTIAL_PROVIDER_USAGE_SCENARIO usage) {
    auto providers = RegisteredProviders();
    std::vector<BOOL> allowed(providers.size(), TRUE);
    const HRESULT hr = filter->Filter(
        usage,
        0,
        providers.data(),
        allowed.data(),
        static_cast<DWORD>(providers.size()));
    std::wcout << L"Filter usage=" << static_cast<int>(usage)
               << L" hr=0x" << std::hex << static_cast<unsigned long>(hr) << std::dec << L"\n";
    CLSID target{};
    CLSIDFromString(kProximityClsid, &target);
    for (size_t index = 0; index < providers.size(); ++index) {
        if (providers[index] == target || !allowed[index]) {
            std::wcout << L"  " << GuidText(providers[index])
                       << L" allowed=" << (allowed[index] ? L"true" : L"false") << L"\n";
        }
    }
}

}  // namespace

int wmain() {
    const HRESULT init = CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED);
    if (FAILED(init)) {
        std::wcerr << L"CoInitializeEx failed: 0x" << std::hex << static_cast<unsigned long>(init) << L"\n";
        return 1;
    }

    CLSID filterClsid{};
    CLSIDFromString(kFilterClsid, &filterClsid);
    ICredentialProviderFilter* filter = nullptr;
    HRESULT hr = CoCreateInstance(
        filterClsid,
        nullptr,
        CLSCTX_INPROC_SERVER,
        IID_PPV_ARGS(&filter));
    std::wcout << L"GenericFilter activation hr=0x" << std::hex
               << static_cast<unsigned long>(hr) << std::dec << L"\n";
    if (SUCCEEDED(hr)) {
        ProbeFilter(filter, CPUS_LOGON);
        ProbeFilter(filter, CPUS_UNLOCK_WORKSTATION);
        filter->Release();
    }

    CLSID proximityClsid{};
    CLSIDFromString(kProximityClsid, &proximityClsid);
    ICredentialProvider* provider = nullptr;
    hr = CoCreateInstance(
        proximityClsid,
        nullptr,
        CLSCTX_INPROC_SERVER,
        IID_PPV_ARGS(&provider));
    std::wcout << L"Proximity provider activation hr=0x" << std::hex
               << static_cast<unsigned long>(hr) << std::dec << L"\n";
    if (SUCCEEDED(hr)) {
        const HRESULT logon = provider->SetUsageScenario(CPUS_LOGON, 0);
        const HRESULT unlock = provider->SetUsageScenario(CPUS_UNLOCK_WORKSTATION, 0);
        std::wcout << L"  SetUsageScenario logon=0x" << std::hex
                   << static_cast<unsigned long>(logon)
                   << L" unlock=0x" << static_cast<unsigned long>(unlock) << std::dec << L"\n";
        provider->Release();
    }

    CoUninitialize();
    return 0;
}

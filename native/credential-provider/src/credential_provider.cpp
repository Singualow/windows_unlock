#include <windows.h>
#include <credentialprovider.h>
#include <ntsecapi.h>
#include <propkey.h>
#include <shlwapi.h>
#include <strsafe.h>

#include <atomic>
#include <cstring>
#include <cstdint>
#include <mutex>
#include <new>
#include <string>
#include <thread>
#include <utility>
#include <vector>

#pragma comment(lib, "advapi32.lib")
#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "secur32.lib")
#pragma comment(lib, "shlwapi.lib")

namespace {

// {C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}
constexpr CLSID CLSID_ProximityUnlock = {
    0xc81fcf2e, 0xb9d0, 0x4eaf, {0x8d, 0x35, 0x55, 0xf7, 0x50, 0xd2, 0x56, 0x1b}};

constexpr wchar_t kProviderName[] = L"Proximity Unlock";
constexpr wchar_t kAuthPipe[] = L"\\\\.\\pipe\\ProximityUnlock.Auth.v1";
constexpr wchar_t kReadyEvent[] = L"Global\\ProximityUnlockCredentialReady";
constexpr wchar_t kCredentialProviderRegistry[] =
    L"SOFTWARE\\Microsoft\\Windows\\CurrentVersion\\Authentication\\Credential Providers\\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}";
constexpr wchar_t kClsidRegistry[] =
    L"SOFTWARE\\Classes\\CLSID\\{C81FCF2E-B9D0-4EAF-8D35-55F750D2561B}";

constexpr DWORD kAuthUnavailable = 0;
constexpr DWORD kAuthAvailable = 1;
constexpr DWORD kAuthError = 2;
constexpr DWORD kAuthInvalid = 3;

constexpr NTSTATUS kStatusSuccess = 0;
constexpr NTSTATUS kStatusLogonFailure = static_cast<NTSTATUS>(0xC000006DL);
constexpr NTSTATUS kStatusWrongPassword = static_cast<NTSTATUS>(0xC000006AL);

HINSTANCE g_instance = nullptr;
std::atomic<long> g_moduleReferences{0};

struct AuthReply {
    DWORD status = kAuthUnavailable;
    std::wstring username;
    std::wstring password;
	std::wstring targetSid;

    ~AuthReply() {
        if (!password.empty()) {
            SecureZeroMemory(password.data(), password.size() * sizeof(wchar_t));
        }
    }
};

DWORD ReadUInt32(const BYTE* value) {
    return static_cast<DWORD>(value[0]) |
           (static_cast<DWORD>(value[1]) << 8) |
           (static_cast<DWORD>(value[2]) << 16) |
           (static_cast<DWORD>(value[3]) << 24);
}

HRESULT CallAuthPipe(const char* command, AuthReply* reply) {
    if (!command || !reply) return E_INVALIDARG;
    std::vector<BYTE> output(32768);
    DWORD bytesRead = 0;
    const DWORD inputLength = static_cast<DWORD>(strlen(command));
    BOOL ok = CallNamedPipeW(
        kAuthPipe,
        const_cast<char*>(command),
        inputLength,
        output.data(),
        static_cast<DWORD>(output.size()),
        &bytesRead,
        3000);
    if (!ok) return HRESULT_FROM_WIN32(GetLastError());
	if (bytesRead < 20 || memcmp(output.data(), "PUA1", 4) != 0) return HRESULT_FROM_WIN32(ERROR_INVALID_DATA);
    reply->status = ReadUInt32(output.data() + 4);
    const DWORD userChars = ReadUInt32(output.data() + 8);
    const DWORD passwordChars = ReadUInt32(output.data() + 12);
	const DWORD sidChars = ReadUInt32(output.data() + 16);
	if (userChars > 4096 || passwordChars > 4096 || sidChars > 4096) return HRESULT_FROM_WIN32(ERROR_INVALID_DATA);
	const size_t expected = 20ULL + 2ULL * userChars + 2ULL * passwordChars + 2ULL * sidChars;
    if (expected != bytesRead) return HRESULT_FROM_WIN32(ERROR_INVALID_DATA);
	const wchar_t* strings = reinterpret_cast<const wchar_t*>(output.data() + 20);
    reply->username.assign(strings, strings + userChars);
    reply->password.assign(strings + userChars, strings + userChars + passwordChars);
	reply->targetSid.assign(strings + userChars + passwordChars, strings + userChars + passwordChars + sidChars);
    SecureZeroMemory(output.data(), output.size());
    return S_OK;
}

HRESULT NotifyAuthPipe(const char* command) {
    AuthReply ignored;
    return CallAuthPipe(command, &ignored);
}

void ReportDiagnostic(const char* eventName) {
    if (!eventName) return;
    char command[96]{};
    if (SUCCEEDED(StringCchPrintfA(command, ARRAYSIZE(command), "DIAG %s\n", eventName))) {
        NotifyAuthPipe(command);
    }
}

HRESULT DuplicateString(const wchar_t* value, PWSTR* output) {
    if (!output) return E_INVALIDARG;
    *output = nullptr;
    return SHStrDupW(value ? value : L"", output);
}

HRESULT DuplicateFieldDescriptor(
    const CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR& source,
    CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR** output) {
    if (!output) return E_INVALIDARG;
    *output = static_cast<CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR*>(
        CoTaskMemAlloc(sizeof(CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR)));
    if (!*output) return E_OUTOFMEMORY;
    **output = source;
    (*output)->pszLabel = nullptr;
    HRESULT hr = DuplicateString(source.pszLabel, &(*output)->pszLabel);
    if (FAILED(hr)) {
        CoTaskMemFree(*output);
        *output = nullptr;
    }
    return hr;
}

HRESULT LookupNegotiatePackage(ULONG* packageId) {
    if (!packageId) return E_INVALIDARG;
    HANDLE lsa = nullptr;
    NTSTATUS status = LsaConnectUntrusted(&lsa);
    if (status != 0) return HRESULT_FROM_WIN32(LsaNtStatusToWinError(status));
    LSA_STRING name{};
    constexpr char negotiate[] = "Negotiate";
    name.Buffer = const_cast<PCHAR>(negotiate);
    name.Length = static_cast<USHORT>(strlen(negotiate));
    name.MaximumLength = name.Length + 1;
    status = LsaLookupAuthenticationPackage(lsa, &name, packageId);
    LsaDeregisterLogonProcess(lsa);
    return status == 0 ? S_OK : HRESULT_FROM_WIN32(LsaNtStatusToWinError(status));
}

void SplitCanonicalUser(const std::wstring& canonical, std::wstring* domain, std::wstring* username) {
    const size_t slash = canonical.find(L'\\');
    if (slash == std::wstring::npos) {
        domain->clear();
        *username = canonical;
    } else {
        *domain = canonical.substr(0, slash);
        *username = canonical.substr(slash + 1);
    }
}

void InitUnicodeString(UNICODE_STRING* value, const std::wstring& text) {
    value->Length = static_cast<USHORT>(text.size() * sizeof(wchar_t));
    value->MaximumLength = static_cast<USHORT>((text.size() + 1) * sizeof(wchar_t));
    value->Buffer = const_cast<PWSTR>(text.c_str());
}

HRESULT PackUnlockLogon(
    CREDENTIAL_PROVIDER_USAGE_SCENARIO usage,
    const std::wstring& canonicalUser,
    const std::wstring& password,
    BYTE** output,
    DWORD* outputSize) {
    if (!output || !outputSize || canonicalUser.empty() || password.empty()) return E_INVALIDARG;
    *output = nullptr;
    *outputSize = 0;
    std::wstring domain;
    std::wstring username;
    SplitCanonicalUser(canonicalUser, &domain, &username);

    KERB_INTERACTIVE_UNLOCK_LOGON unpacked{};
    unpacked.Logon.MessageType = usage == CPUS_UNLOCK_WORKSTATION
        ? KerbWorkstationUnlockLogon
        : KerbInteractiveLogon;
    InitUnicodeString(&unpacked.Logon.LogonDomainName, domain);
    InitUnicodeString(&unpacked.Logon.UserName, username);
    InitUnicodeString(&unpacked.Logon.Password, password);

    const size_t domainBytes = (domain.size() + 1) * sizeof(wchar_t);
    const size_t usernameBytes = (username.size() + 1) * sizeof(wchar_t);
    const size_t passwordBytes = (password.size() + 1) * sizeof(wchar_t);
    const size_t total = sizeof(KERB_INTERACTIVE_UNLOCK_LOGON) + domainBytes + usernameBytes + passwordBytes;
    if (total > MAXDWORD) return HRESULT_FROM_WIN32(ERROR_ARITHMETIC_OVERFLOW);
    BYTE* packed = static_cast<BYTE*>(CoTaskMemAlloc(total));
    if (!packed) return E_OUTOFMEMORY;
    SecureZeroMemory(packed, total);
    auto* logon = reinterpret_cast<KERB_INTERACTIVE_UNLOCK_LOGON*>(packed);
    *logon = unpacked;
    BYTE* cursor = packed + sizeof(KERB_INTERACTIVE_UNLOCK_LOGON);
    auto copyRelative = [&](UNICODE_STRING* target, const std::wstring& text, size_t bytes) {
        memcpy(cursor, text.c_str(), bytes);
        target->Buffer = reinterpret_cast<PWSTR>(cursor - packed);
        cursor += bytes;
    };
    copyRelative(&logon->Logon.LogonDomainName, domain, domainBytes);
    copyRelative(&logon->Logon.UserName, username, usernameBytes);
    copyRelative(&logon->Logon.Password, password, passwordBytes);
    *output = packed;
    *outputSize = static_cast<DWORD>(total);
    return S_OK;
}

enum FieldId : DWORD {
    FieldTitle = 0,
    FieldStatus = 1,
    FieldCount = 2,
};

CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR g_fields[FieldCount] = {
    {FieldTitle, CPFT_LARGE_TEXT, const_cast<PWSTR>(L"蓝牙手机")},
    {FieldStatus, CPFT_SMALL_TEXT, const_cast<PWSTR>(L"手机已通过加密认证，正在解锁…")},
};

class Credential final : public ICredentialProviderCredential2 {
public:
    Credential(CREDENTIAL_PROVIDER_USAGE_SCENARIO usage, std::wstring sid)
        : usage_(usage), sid_(std::move(sid)) {
        g_moduleReferences.fetch_add(1);
    }

    ~Credential() {
        if (events_) events_->Release();
        g_moduleReferences.fetch_sub(1);
    }

    IFACEMETHODIMP QueryInterface(REFIID riid, void** value) override {
        if (!value) return E_INVALIDARG;
        *value = nullptr;
        if (riid == IID_IUnknown || riid == IID_ICredentialProviderCredential) {
            *value = static_cast<ICredentialProviderCredential*>(this);
        } else if (riid == IID_ICredentialProviderCredential2) {
            *value = static_cast<ICredentialProviderCredential2*>(this);
        } else {
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override { return static_cast<ULONG>(references_.fetch_add(1) + 1); }
    IFACEMETHODIMP_(ULONG) Release() override {
        const long remaining = references_.fetch_sub(1) - 1;
        if (remaining == 0) delete this;
        return static_cast<ULONG>(remaining);
    }

    IFACEMETHODIMP Advise(ICredentialProviderCredentialEvents* events) override {
        if (!events) return E_INVALIDARG;
        if (events_) events_->Release();
        events_ = events;
        events_->AddRef();
        return S_OK;
    }

    IFACEMETHODIMP UnAdvise() override {
        if (events_) {
            events_->Release();
            events_ = nullptr;
        }
        return S_OK;
    }

    IFACEMETHODIMP SetSelected(BOOL* autoLogon) override {
        if (!autoLogon) return E_INVALIDARG;
        AuthReply reply;
        const HRESULT hr = CallAuthPipe("PEEK\n", &reply);
        *autoLogon = SUCCEEDED(hr) && reply.status == kAuthAvailable ? TRUE : FALSE;
        ReportDiagnostic(*autoLogon ? "SELECTED_AUTOLOGON" : "SELECTED_NO_AUTH");
        return S_OK;
    }

    IFACEMETHODIMP SetDeselected() override { return S_OK; }

    IFACEMETHODIMP GetFieldState(
        DWORD fieldId,
        CREDENTIAL_PROVIDER_FIELD_STATE* state,
        CREDENTIAL_PROVIDER_FIELD_INTERACTIVE_STATE* interactive) override {
        if (!state || !interactive || fieldId >= FieldCount) return E_INVALIDARG;
        *state = CPFS_DISPLAY_IN_SELECTED_TILE;
        *interactive = CPFIS_NONE;
        return S_OK;
    }

    IFACEMETHODIMP GetStringValue(DWORD fieldId, PWSTR* value) override {
        if (fieldId >= FieldCount) return E_INVALIDARG;
        return DuplicateString(g_fields[fieldId].pszLabel, value);
    }

    IFACEMETHODIMP GetBitmapValue(DWORD, HBITMAP*) override { return E_NOTIMPL; }
    IFACEMETHODIMP GetCheckboxValue(DWORD, BOOL*, PWSTR*) override { return E_NOTIMPL; }
    IFACEMETHODIMP GetSubmitButtonValue(DWORD, DWORD*) override { return E_NOTIMPL; }
    IFACEMETHODIMP GetComboBoxValueCount(DWORD, DWORD*, DWORD*) override { return E_NOTIMPL; }
    IFACEMETHODIMP GetComboBoxValueAt(DWORD, DWORD, PWSTR*) override { return E_NOTIMPL; }
    IFACEMETHODIMP SetStringValue(DWORD, PCWSTR) override { return E_NOTIMPL; }
    IFACEMETHODIMP SetCheckboxValue(DWORD, BOOL) override { return E_NOTIMPL; }
    IFACEMETHODIMP SetComboBoxSelectedValue(DWORD, DWORD) override { return E_NOTIMPL; }
    IFACEMETHODIMP CommandLinkClicked(DWORD) override { return E_NOTIMPL; }

    IFACEMETHODIMP GetSerialization(
        CREDENTIAL_PROVIDER_GET_SERIALIZATION_RESPONSE* response,
        CREDENTIAL_PROVIDER_CREDENTIAL_SERIALIZATION* serialization,
        PWSTR* statusText,
        CREDENTIAL_PROVIDER_STATUS_ICON* statusIcon) override {
        if (!response || !serialization || !statusText || !statusIcon) return E_INVALIDARG;
        *response = CPGSR_NO_CREDENTIAL_NOT_FINISHED;
        *statusText = nullptr;
        *statusIcon = CPSI_NONE;
        ZeroMemory(serialization, sizeof(*serialization));
		ReportDiagnostic("SERIALIZATION_START");

        AuthReply auth;
        HRESULT hr = CallAuthPipe("CONSUME\n", &auth);
        if (FAILED(hr) || auth.status != kAuthAvailable || auth.username.empty() || auth.password.empty()) {
			ReportDiagnostic("CONSUME_UNAVAILABLE");
            DuplicateString(L"手机授权已过期，请重新靠近。", statusText);
            return S_OK;
        }
        BYTE* packed = nullptr;
        DWORD packedSize = 0;
        hr = PackUnlockLogon(usage_, auth.username, auth.password, &packed, &packedSize);
        if (FAILED(hr)) return hr;
        ULONG packageId = 0;
        hr = LookupNegotiatePackage(&packageId);
        if (FAILED(hr)) {
            SecureZeroMemory(packed, packedSize);
            CoTaskMemFree(packed);
            return hr;
        }
        serialization->ulAuthenticationPackage = packageId;
        serialization->clsidCredentialProvider = CLSID_ProximityUnlock;
        serialization->cbSerialization = packedSize;
        serialization->rgbSerialization = packed;
        *response = CPGSR_RETURN_CREDENTIAL_FINISHED;
		ReportDiagnostic("SERIALIZATION_READY");
        return S_OK;
    }

    IFACEMETHODIMP ReportResult(
        NTSTATUS status,
        NTSTATUS,
        PWSTR* statusText,
        CREDENTIAL_PROVIDER_STATUS_ICON* statusIcon) override {
        if (!statusText || !statusIcon) return E_INVALIDARG;
        *statusText = nullptr;
        *statusIcon = CPSI_NONE;
        if (status == kStatusSuccess) {
            NotifyAuthPipe("SUCCESS\n");
		} else {
            NotifyAuthPipe("INVALID\n");
            DuplicateString(L"Windows 密码已失效。请使用 PIN/密码登录后重新录入。", statusText);
            *statusIcon = CPSI_ERROR;
        }
        return S_OK;
    }

    IFACEMETHODIMP GetUserSid(PWSTR* sid) override { return DuplicateString(sid_.c_str(), sid); }

private:
    std::atomic<long> references_{1};
    CREDENTIAL_PROVIDER_USAGE_SCENARIO usage_ = CPUS_INVALID;
    std::wstring sid_;
    ICredentialProviderCredentialEvents* events_ = nullptr;
};

class Provider final : public ICredentialProvider, public ICredentialProviderSetUserArray {
public:
    Provider() { g_moduleReferences.fetch_add(1); }

    ~Provider() {
        StopWatcher();
        if (events_) events_->Release();
        if (users_) users_->Release();
        if (credential_) credential_->Release();
        g_moduleReferences.fetch_sub(1);
    }

    IFACEMETHODIMP QueryInterface(REFIID riid, void** value) override {
        if (!value) return E_INVALIDARG;
        *value = nullptr;
        if (riid == IID_IUnknown || riid == IID_ICredentialProvider) {
            *value = static_cast<ICredentialProvider*>(this);
        } else if (riid == IID_ICredentialProviderSetUserArray) {
            *value = static_cast<ICredentialProviderSetUserArray*>(this);
        } else {
            return E_NOINTERFACE;
        }
        AddRef();
        return S_OK;
    }

    IFACEMETHODIMP_(ULONG) AddRef() override { return static_cast<ULONG>(references_.fetch_add(1) + 1); }
    IFACEMETHODIMP_(ULONG) Release() override {
        const long remaining = references_.fetch_sub(1) - 1;
        if (remaining == 0) delete this;
        return static_cast<ULONG>(remaining);
    }

    IFACEMETHODIMP SetUsageScenario(CREDENTIAL_PROVIDER_USAGE_SCENARIO usage, DWORD) override {
        if (usage != CPUS_LOGON && usage != CPUS_UNLOCK_WORKSTATION) return E_NOTIMPL;
        usage_ = usage;
        return S_OK;
    }

    IFACEMETHODIMP SetSerialization(const CREDENTIAL_PROVIDER_CREDENTIAL_SERIALIZATION*) override {
        return E_NOTIMPL;
    }

    IFACEMETHODIMP Advise(ICredentialProviderEvents* events, UINT_PTR context) override {
        if (!events) return E_INVALIDARG;
        std::lock_guard lock(mutex_);
        if (events_) events_->Release();
        events_ = events;
        events_->AddRef();
        adviseContext_ = context;
        StartWatcher();
        return S_OK;
    }

    IFACEMETHODIMP UnAdvise() override {
        StopWatcher();
        std::lock_guard lock(mutex_);
        if (events_) {
            events_->Release();
            events_ = nullptr;
        }
        return S_OK;
    }

    IFACEMETHODIMP GetFieldDescriptorCount(DWORD* count) override {
        if (!count) return E_INVALIDARG;
        *count = FieldCount;
        return S_OK;
    }

    IFACEMETHODIMP GetFieldDescriptorAt(DWORD index, CREDENTIAL_PROVIDER_FIELD_DESCRIPTOR** descriptor) override {
        if (index >= FieldCount) return E_INVALIDARG;
        return DuplicateFieldDescriptor(g_fields[index], descriptor);
    }

    IFACEMETHODIMP GetCredentialCount(DWORD* count, DWORD* defaultIndex, BOOL* autoLogon) override {
        if (!count || !defaultIndex || !autoLogon) return E_INVALIDARG;
        *count = 0;
        *defaultIndex = CREDENTIAL_PROVIDER_NO_DEFAULT;
        *autoLogon = FALSE;
        AuthReply auth;
        HRESULT hr = CallAuthPipe("PEEK\n", &auth);
		if (FAILED(hr) || auth.status != kAuthAvailable || auth.username.empty() || auth.targetSid.empty()) return S_OK;
		// The service already binds the one-time grant to the active locked
		// console session and enrolled SID. Some Windows 11 Microsoft-account
		// lock screens provide an empty or incomplete V2 user array. Prefer the
		// enumerated SID when available, but safely fall back to the service-
		// validated target SID instead of suppressing the credential entirely.
		std::wstring sid = auth.targetSid;
		std::wstring enumeratedSid;
		if (users_ && SUCCEEDED(FindUser(auth.targetSid, &enumeratedSid)) && !enumeratedSid.empty()) {
			sid = std::move(enumeratedSid);
			ReportDiagnostic("TARGET_USER_MATCHED");
		} else {
			ReportDiagnostic("TARGET_SID_FALLBACK");
		}
        if (credential_) {
            credential_->Release();
            credential_ = nullptr;
        }
        credential_ = new (std::nothrow) Credential(usage_, std::move(sid));
        if (!credential_) return E_OUTOFMEMORY;
        *count = 1;
        *defaultIndex = 0;
        *autoLogon = TRUE;
		ReportDiagnostic("CREDENTIAL_CREATED");
        return S_OK;
    }

    IFACEMETHODIMP GetCredentialAt(DWORD index, ICredentialProviderCredential** credential) override {
        if (!credential || index != 0 || !credential_) return E_INVALIDARG;
        *credential = credential_;
        credential_->AddRef();
		ReportDiagnostic("CREDENTIAL_RETURNED");
        return S_OK;
    }

    IFACEMETHODIMP SetUserArray(ICredentialProviderUserArray* users) override {
        if (!users) return E_INVALIDARG;
        if (users_) users_->Release();
        users_ = users;
        users_->AddRef();
        return S_OK;
    }

private:
	HRESULT FindUser(const std::wstring& targetSid, std::wstring* sid) {
        DWORD count = 0;
        HRESULT hr = users_->GetCount(&count);
        if (FAILED(hr)) return hr;
        for (DWORD index = 0; index < count; ++index) {
            ICredentialProviderUser* user = nullptr;
            hr = users_->GetAt(index, &user);
            if (FAILED(hr)) continue;
            PWSTR candidateSid = nullptr;
            const HRESULT sidHr = user->GetSid(&candidateSid);
			if (SUCCEEDED(sidHr) && candidateSid && _wcsicmp(candidateSid, targetSid.c_str()) == 0) {
                *sid = candidateSid;
            }
            CoTaskMemFree(candidateSid);
            user->Release();
            if (!sid->empty()) return S_OK;
        }
        return HRESULT_FROM_WIN32(ERROR_NOT_FOUND);
    }

    void StartWatcher() {
        if (watcher_.joinable()) return;
        stopEvent_ = CreateEventW(nullptr, TRUE, FALSE, nullptr);
        readyEvent_ = CreateEventW(nullptr, FALSE, FALSE, kReadyEvent);
        if (!stopEvent_ || !readyEvent_) return;
        watcher_ = std::thread([this] {
            CoInitializeEx(nullptr, COINIT_MULTITHREADED);
            HANDLE events[2] = {stopEvent_, readyEvent_};
            while (WaitForMultipleObjects(2, events, FALSE, INFINITE) == WAIT_OBJECT_0 + 1) {
                std::lock_guard lock(mutex_);
                if (events_) events_->CredentialsChanged(adviseContext_);
            }
            CoUninitialize();
        });
    }

    void StopWatcher() {
        if (stopEvent_) SetEvent(stopEvent_);
        if (watcher_.joinable()) watcher_.join();
        if (stopEvent_) CloseHandle(stopEvent_);
        if (readyEvent_) CloseHandle(readyEvent_);
        stopEvent_ = nullptr;
        readyEvent_ = nullptr;
    }

    std::atomic<long> references_{1};
    CREDENTIAL_PROVIDER_USAGE_SCENARIO usage_ = CPUS_INVALID;
    ICredentialProviderEvents* events_ = nullptr;
    ICredentialProviderUserArray* users_ = nullptr;
    Credential* credential_ = nullptr;
    UINT_PTR adviseContext_ = 0;
    std::mutex mutex_;
    HANDLE stopEvent_ = nullptr;
    HANDLE readyEvent_ = nullptr;
    std::thread watcher_;
};

class ClassFactory final : public IClassFactory {
public:
    ClassFactory() { g_moduleReferences.fetch_add(1); }
    ~ClassFactory() { g_moduleReferences.fetch_sub(1); }

    IFACEMETHODIMP QueryInterface(REFIID riid, void** value) override {
        if (!value) return E_INVALIDARG;
        *value = nullptr;
        if (riid != IID_IUnknown && riid != IID_IClassFactory) return E_NOINTERFACE;
        *value = static_cast<IClassFactory*>(this);
        AddRef();
        return S_OK;
    }
    IFACEMETHODIMP_(ULONG) AddRef() override { return static_cast<ULONG>(references_.fetch_add(1) + 1); }
    IFACEMETHODIMP_(ULONG) Release() override {
        const long remaining = references_.fetch_sub(1) - 1;
        if (remaining == 0) delete this;
        return static_cast<ULONG>(remaining);
    }
    IFACEMETHODIMP CreateInstance(IUnknown* outer, REFIID riid, void** value) override {
        if (outer) return CLASS_E_NOAGGREGATION;
        auto* provider = new (std::nothrow) Provider();
        if (!provider) return E_OUTOFMEMORY;
        const HRESULT hr = provider->QueryInterface(riid, value);
        provider->Release();
        return hr;
    }
    IFACEMETHODIMP LockServer(BOOL lock) override {
        if (lock) g_moduleReferences.fetch_add(1);
        else g_moduleReferences.fetch_sub(1);
        return S_OK;
    }
private:
    std::atomic<long> references_{1};
};

HRESULT SetRegistryString(HKEY root, const wchar_t* path, const wchar_t* name, const wchar_t* value) {
    HKEY key = nullptr;
    LONG result = RegCreateKeyExW(root, path, 0, nullptr, 0, KEY_WRITE, nullptr, &key, nullptr);
    if (result != ERROR_SUCCESS) return HRESULT_FROM_WIN32(result);
    const DWORD bytes = static_cast<DWORD>((wcslen(value) + 1) * sizeof(wchar_t));
    result = RegSetValueExW(key, name, 0, REG_SZ, reinterpret_cast<const BYTE*>(value), bytes);
    RegCloseKey(key);
    return HRESULT_FROM_WIN32(result);
}

}  // namespace

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        g_instance = instance;
        DisableThreadLibraryCalls(instance);
    }
    return TRUE;
}

extern "C" HRESULT __stdcall DllCanUnloadNow() {
    return g_moduleReferences.load() == 0 ? S_OK : S_FALSE;
}

extern "C" HRESULT __stdcall DllGetClassObject(REFCLSID clsid, REFIID riid, void** value) {
    if (clsid != CLSID_ProximityUnlock) return CLASS_E_CLASSNOTAVAILABLE;
    auto* factory = new (std::nothrow) ClassFactory();
    if (!factory) return E_OUTOFMEMORY;
    const HRESULT hr = factory->QueryInterface(riid, value);
    factory->Release();
    return hr;
}

extern "C" HRESULT __stdcall DllRegisterServer() {
    wchar_t modulePath[MAX_PATH]{};
    if (!GetModuleFileNameW(g_instance, modulePath, ARRAYSIZE(modulePath))) {
        return HRESULT_FROM_WIN32(GetLastError());
    }
    HRESULT hr = SetRegistryString(HKEY_LOCAL_MACHINE, kCredentialProviderRegistry, nullptr, kProviderName);
    if (FAILED(hr)) return hr;
    hr = SetRegistryString(HKEY_LOCAL_MACHINE, kClsidRegistry, nullptr, kProviderName);
    if (FAILED(hr)) return hr;
    std::wstring inproc = std::wstring(kClsidRegistry) + L"\\InprocServer32";
    hr = SetRegistryString(HKEY_LOCAL_MACHINE, inproc.c_str(), nullptr, modulePath);
    if (FAILED(hr)) return hr;
    return SetRegistryString(HKEY_LOCAL_MACHINE, inproc.c_str(), L"ThreadingModel", L"Apartment");
}

extern "C" HRESULT __stdcall DllUnregisterServer() {
    RegDeleteTreeW(HKEY_LOCAL_MACHINE, kCredentialProviderRegistry);
    RegDeleteTreeW(HKEY_LOCAL_MACHINE, kClsidRegistry);
    return S_OK;
}

cmake_minimum_required(VERSION 3.24)
project(ProximityUnlockCredentialProvider LANGUAGES CXX RC)

add_library(ProximityUnlockCredentialProvider SHARED
    src/credential_provider.cpp
    src/credential_provider.def
)

target_compile_features(ProximityUnlockCredentialProvider PRIVATE cxx_std_20)
target_compile_definitions(ProximityUnlockCredentialProvider PRIVATE
    UNICODE _UNICODE WIN32_LEAN_AND_MEAN NOMINMAX
)
target_compile_options(ProximityUnlockCredentialProvider PRIVATE
    /W4 /WX /utf-8 /guard:cf /sdl /permissive- /Zc:__cplusplus
)
target_link_options(ProximityUnlockCredentialProvider PRIVATE /guard:cf)
target_link_libraries(ProximityUnlockCredentialProvider PRIVATE
    advapi32 ole32 secur32 shlwapi
)

add_executable(CredentialProviderProbe
    src/credential_provider_probe.cpp
)
target_compile_features(CredentialProviderProbe PRIVATE cxx_std_20)
target_compile_definitions(CredentialProviderProbe PRIVATE
    UNICODE _UNICODE WIN32_LEAN_AND_MEAN NOMINMAX
)
target_compile_options(CredentialProviderProbe PRIVATE
    /W4 /utf-8 /permissive- /EHsc
)
target_link_libraries(CredentialProviderProbe PRIVATE advapi32 ole32)

set_target_properties(ProximityUnlockCredentialProvider PROPERTIES
    OUTPUT_NAME "ProximityUnlockCredentialProvider"
)

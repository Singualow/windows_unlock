plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

val personalStorePath = System.getenv("PROXIMITY_UNLOCK_KEYSTORE")
val personalStorePassword = System.getenv("PROXIMITY_UNLOCK_STORE_PASSWORD")
val personalKeyAlias = System.getenv("PROXIMITY_UNLOCK_KEY_ALIAS")
val personalKeyPassword = System.getenv("PROXIMITY_UNLOCK_KEY_PASSWORD")

android {
    namespace = "com.singu.proximityunlock"
    compileSdk = 35

    defaultConfig {
        applicationId = "com.singu.proximityunlock"
        minSdk = 35
        targetSdk = 35
        versionCode = 7
        versionName = "0.2.0"
    }

	signingConfigs {
		if (!personalStorePath.isNullOrBlank() && !personalStorePassword.isNullOrBlank() &&
			!personalKeyAlias.isNullOrBlank() && !personalKeyPassword.isNullOrBlank()) {
			create("personal") {
				storeFile = file(personalStorePath)
				storePassword = personalStorePassword
				keyAlias = personalKeyAlias
				keyPassword = personalKeyPassword
			}
		}
	}

    buildTypes {
        release {
            isMinifyEnabled = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
			signingConfig = signingConfigs.findByName("personal")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions { jvmTarget = "17" }
}

dependencies {
	testImplementation("junit:junit:4.13.2")
}

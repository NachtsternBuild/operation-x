plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "de.operationx.app"
    compileSdk = 35

    defaultConfig {
        applicationId = "de.operationx.app"
        // Android 8 deckt praktisch jedes Gerät ab, das noch im Umlauf ist.
        // Material You mit dynamischer Farbe braucht 12, darunter greift das
        // feste Farbschema.
        minSdk = 26
        targetSdk = 35
        versionCode = 1
        versionName = "0.1"

        // Die Kartenbibliothek bringt native Bibliotheken für vier
        // Architekturen mit und verdoppelt damit die Größe. x86 gibt es nur in
        // Emulatoren – echte Telefone sind seit Jahren ARM. Das halbiert den
        // Download, der am Spieltag oft über Mobilfunk läuft.
        ndk {
            abiFilters += listOf("arm64-v8a", "armeabi-v7a")
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
    }

    sourceSets["main"].java.srcDirs("src/main/kotlin")
    sourceSets["test"].java.srcDirs("src/test/kotlin")
}

dependencies {
    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.service)
    implementation(libs.androidx.lifecycle.compose)
    implementation(libs.androidx.activity.compose)

    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.ui)
    implementation(libs.androidx.ui.graphics)
    implementation(libs.androidx.ui.tooling.preview)
    implementation(libs.androidx.material3)
    implementation(libs.androidx.material.icons)
    implementation(libs.androidx.navigation.compose)

    implementation(libs.androidx.datastore)
    implementation(libs.okhttp)
    implementation(libs.kotlinx.serialization.json)

    // Die Karte. MapLibre Native rendert dieselben Vektordaten wie die
    // Weboberfläche, nur eben auf dem Gerät.
    implementation(libs.maplibre)

    // Für den QR-Beitritt.
    implementation(libs.camerax.camera2)
    implementation(libs.camerax.lifecycle)
    implementation(libs.camerax.view)
    implementation(libs.zxing.core)

    debugImplementation(libs.androidx.ui.tooling)

    // Für die Teile, die ohne Gerät prüfbar sind. Der Prüfdurchgang hat
    // gezeigt, warum das nötig ist: Die Regel, welche Adresse unverschlüsselt
    // erreicht werden darf, entscheidet darüber, ob Zugangsdaten im Klartext
    // durchs Mobilfunknetz gehen. So etwas gehört nicht auf ein Telefon
    // getippt, sondern in einen Test.
    testImplementation(libs.junit)
}

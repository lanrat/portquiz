plugins {
    id("com.android.application")
}

// The release version, passed as -PportquizVersion=v1.4.0 when building a
// release; local builds are "dev".
val portquizVersion = (findProperty("portquizVersion") as String?)?.removePrefix("v") ?: "dev"

// versionCode must grow with every release for Android to install it as an
// update: 1.4.0 becomes 10400. Local builds use 1.
fun versionCodeOf(version: String): Int {
    val parts = version.split(".").map { it.toIntOrNull() }
    if (parts.size != 3 || parts.any { it == null }) return 1
    val (major, minor, patch) = parts.map { it!! }
    return major * 10000 + minor * 100 + patch
}

android {
    namespace = "io.github.lanrat.portquiz"
    compileSdk = 37
    ndkVersion = "30.0.16248370"

    defaultConfig {
        applicationId = "io.github.lanrat.portquiz"
        minSdk = 29
        // Raise to 37 after testing on Android 17, whose behavior changes
        // for apps targeting it have not been tried with this app yet.
        targetSdk = 36
        versionCode = versionCodeOf(portquizVersion)
        versionName = portquizVersion
        ndk {
            abiFilters += "arm64-v8a"
        }
    }

    // Release builds are signed when PORTQUIZ_KEYSTORE points at a keystore
    // and PORTQUIZ_KEYSTORE_PASSWORD holds its password, as the release
    // workflow does; otherwise they are left unsigned. The key alias defaults
    // to "portquiz" and the key password to the keystore password, which is
    // how keytool creates a PKCS12 keystore (see android/README.md).
    val keystore = System.getenv("PORTQUIZ_KEYSTORE")
    signingConfigs {
        if (keystore != null) {
            create("release") {
                val password = System.getenv("PORTQUIZ_KEYSTORE_PASSWORD")
                storeFile = file(keystore)
                storePassword = password
                keyAlias = System.getenv("PORTQUIZ_KEY_ALIAS") ?: "portquiz"
                keyPassword = System.getenv("PORTQUIZ_KEY_PASSWORD") ?: password
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
            if (keystore != null) {
                signingConfig = signingConfigs.getByName("release")
            }
        }
    }

    buildFeatures {
        buildConfig = true
    }

    testOptions {
        // ScanState reads SystemClock, which plain JVM tests stub out
        unitTests.isReturnDefaultValues = true
    }

    lint {
        warningsAsErrors = true
        lintConfig = file("lint.xml")
    }
}

kotlin {
    compilerOptions {
        allWarningsAsErrors = true
    }
}

// Build the Go scanning library with gomobile before compiling the app.
val goAar = layout.buildDirectory.file("go/portquiz.aar")
val goDir = rootProject.file("gomobile")
val bindGo = tasks.register<Exec>("bindGo") {
    description = "Builds the Go scanning library with gomobile."
    inputs.files(fileTree(goDir) {
        include("*.go", "go.mod", "go.sum", "bind.sh")
    })
    inputs.files(fileTree(rootProject.file("../scan")) { include("*.go") })
    inputs.files(rootProject.file("../go.mod"), rootProject.file("../go.sum"))
    outputs.file(goAar)

    val sdkDir = androidComponents.sdkComponents.sdkDirectory
    val ndkDir = androidComponents.sdkComponents.ndkDirectory
    commandLine("sh", goDir.resolve("bind.sh").path, goAar.get().asFile.path)
    doFirst {
        environment("ANDROID_HOME", sdkDir.get().asFile.path)
        environment("ANDROID_NDK_HOME", ndkDir.get().asFile.path)
    }
}

dependencies {
    implementation(files(goAar).builtBy(bindGo))
    testImplementation("junit:junit:4.13.2")
}

package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"signalserver/internal/config"

	"github.com/sirupsen/logrus"
)

var Log *logrus.Logger

// InitLogger inicializa el logger de la aplicación basado en la configuración.
func InitLogger(cfg *config.AppConfig) {
	Log = logrus.New()

	// Configurar el nivel de log
	level, err := logrus.ParseLevel(cfg.ConsoleLogLevel)
	if err != nil {
		Log.Warnf("Nivel de log inválido '%s' para consola, usando 'info' por defecto.", cfg.ConsoleLogLevel)
		Log.SetLevel(logrus.InfoLevel)
	} else {
		Log.SetLevel(level)
	}

	// Configurar la salida de la consola
	// Si ConsoleVerbose es true, la salida va a stdout con colores.
	// Si ConsoleVerbose es false, la salida del logger se descarta.
	if cfg.ConsoleVerbose {
		Log.SetOutput(os.Stdout)
		Log.SetFormatter(&logrus.TextFormatter{
			ForceColors:     true,
			FullTimestamp:   true,
			TimestampFormat: "2006-01-02 15:04:05",
		})
	} else {
		Log.SetOutput(io.Discard) // Descartar salida del logger si no es verbose en consola
	}

	// Mostrar el banner si está habilitado
	if cfg.ShowBanner {
		displayBanner()
	}
}

// displayBanner lee y muestra el contenido de banner.txt.
func displayBanner() {
	// Obtener el directorio de trabajo actual para construir la ruta relativa
	wd, err := os.Getwd()
	if err != nil {
		Log.Warnf("No se pudo obtener el directorio de trabajo actual para el banner: %v", err)
		return
	}
	bannerPath := filepath.Join(wd, "banner.txt")

	content, err := os.ReadFile(bannerPath)
	if err != nil {
		Log.Warnf("No se pudo leer el archivo banner.txt en %s: %v", bannerPath, err)
		return
	}
	fmt.Println(string(content))
}

// Helper functions para niveles de log específicos.
// Logrus ya maneja colores para Info, Warn, Error, Debug, Fatal con TextFormatter y ForceColors: true.
// Para "SUCCESS", creamos una función que usa el nivel Info pero con un prefijo y color explícito.

const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
)

// Exported colors for use in other packages
const (
	ColorRed    = colorRed
	ColorGreen  = colorGreen
	ColorYellow = colorYellow
	ColorBlue   = colorBlue
	ColorPurple = colorPurple
	ColorCyan   = colorCyan
	ColorReset  = colorReset
)

func Info(args ...interface{}) {
	Log.Info(args...)
}

func Warn(args ...interface{}) {
	Log.Warn(args...)
}

func Error(args ...interface{}) {
	Log.Error(args...)
}

func Debug(args ...interface{}) {
	Log.Debug(args...)
}

func Fatal(args ...interface{}) {
	Log.Fatal(args...)
}

// Success loguea un mensaje con un indicador de éxito en verde.
func Success(format string, args ...interface{}) {
	Log.Infof(colorGreen+"SUCCESS: "+format+colorReset, args...)
}

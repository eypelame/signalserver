@echo off
cls

:MENU
echo.---------------------------------------------------
echo.  Seleccione el sistema operativo de destino para compilar:
echo.---------------------------------------------------
echo.  1. Windows (signalserver.exe)
echo.  2. Linux (signalserver)
echo.  3. Salir
echo.---------------------------------------------------
set /p choice="Ingrese su opcion (1, 2 o 3): "

if "%choice%"=="1" goto COMPILE_WINDOWS
if "%choice%"=="2" goto COMPILE_LINUX
if "%choice%"=="3" goto END
echo.Opcion invalida. Por favor, intente de nuevo.
echo.
goto MENU

:COMPILE_WINDOWS
echo.Compilando para Windows...
set GOOS=windows
set GOARCH=amd64
go build -o signalserver.exe ./cmd/server
if %errorlevel% equ 0 (
    echo.Compilacion para Windows exitosa: signalserver.exe
) else (
    echo.Error durante la compilacion para Windows.
)
goto CLEANUP

:COMPILE_LINUX
echo.Compilando para Linux...
set GOOS=linux
set GOARCH=amd64
go build -o signalserver ./cmd/server
if %errorlevel% equ 0 (
    echo.Compilacion para Linux exitosa: signalserver
) else (
    echo.Error durante la compilacion para Linux.
)
goto CLEANUP

:CLEANUP
rem Limpiar las variables de entorno GOOS y GOARCH para no afectar futuras compilaciones
set GOOS=
set GOARCH=
echo.Presione cualquier tecla para continuar...
pause > nul
goto END

:END
echo.Saliendo del compilador.

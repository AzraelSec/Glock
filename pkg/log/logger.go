package log

type Logger interface {
	Error(format string, args ...any) (int, error)
	Info(format string, args ...any) (int, error)
	Success(format string, args ...any) (int, error)
	Print(format string, args ...any) (int, error)
}

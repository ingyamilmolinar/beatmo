/* Explicit gcov counter dump for coverage-instrumented builds.
 *
 * The Go runtime exits via the exit_group syscall without running C atexit
 * handlers or destructors, so libgcov's normal exit-time flush never fires
 * in a cgo binary and an instrumented run would silently produce no .gcda
 * files. scripts/coverage-c.sh compiles this file with -DBEATMO_GCOV so
 * beatmo_gcov_flush() strongly references __gcov_dump — a strong reference
 * is required because a weak one does not pull the defining member out of
 * libgcov.a. The normal build (Makefile $(C_LIB) rule) compiles the no-op
 * variant, keeping production binaries free of any libgcov dependency.
 *
 * Called from Go via audio.FlushCCoverage() in TestMain (see
 * src/go/internal/audio/main_native_test.go).
 */
#ifdef BEATMO_GCOV
extern void __gcov_dump(void);
void beatmo_gcov_flush(void) { __gcov_dump(); }
#else
void beatmo_gcov_flush(void) {}
#endif

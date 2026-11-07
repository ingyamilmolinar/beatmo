/* The Go runtime exits without C atexit handlers, so libgcov never flushes in a cgo binary. With -DBEATMO_GCOV this
 * strongly references __gcov_dump (a weak reference would not pull it out of libgcov.a); the normal build compiles
 * the no-op, so production carries no libgcov. */
#ifdef BEATMO_GCOV
extern void __gcov_dump(void);
void beatmo_gcov_flush(void) { __gcov_dump(); }
#else
void beatmo_gcov_flush(void) {}
#endif

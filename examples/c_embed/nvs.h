/* nvs.h — NvS (Navescript) C embedding API.
 *
 * Build the shared library from the NvS repo:
 *
 *   go build -buildmode=c-shared -o libnvs.so ./cbridge
 *
 * That also generates a cgo libnvs.h; this hand-written header is the
 * stable, documented contract — the signatures are identical.
 *
 * Threading / lifetime contract:
 *   - One global interpreter per process. All calls are serialized on a
 *     mutex; there is exactly one persistent NvS environment.
 *   - nvs_eval evaluates a program string in that environment. Functions
 *     and bindings defined by one call are visible to later calls.
 *   - nvs_call applies a named NvS function (or builtin) to a JSON array
 *     of arguments. Arguments are positional only.
 *   - Both return a freshly allocated JSON envelope string:
 *         {"ok":true,"result":<json>}   on success
 *         {"ok":false,"error":"..."}    on failure
 *     <json> covers NvS int, float, string, bool, null, array, and hash.
 *     Any other NvS value (function, class instance, channel, ...) yields
 *     ok:false naming the type — values are never silently mis-converted.
 *   - The caller MUST pass every returned string to nvs_free exactly once.
 *     The input strings (src, func_name, args_json) are borrowed; the
 *     bridge copies what it needs before returning.
 *   - A non-terminating NvS program blocks the calling thread.
 *     NvS `print` output goes to the host process's stdout.
 *
 * Compile a client:
 *
 *   gcc -o embed main.c -L. -lnvs -Wl,-rpath,'$ORIGIN'
 */
#ifndef NVS_H
#define NVS_H

#ifdef __cplusplus
extern "C" {
#endif

/* Evaluate NvS source in the persistent environment.
 * Returns a JSON envelope; free with nvs_free. */
char *nvs_eval(const char *src);

/* Call a named NvS function with JSON-encoded args, e.g. "[20, 22]".
 * Returns a JSON envelope; free with nvs_free. */
char *nvs_call(const char *func_name, const char *args_json);

/* Free a string returned by nvs_eval / nvs_call. Call exactly once. */
void nvs_free(char *s);

#ifdef __cplusplus
}
#endif

#endif /* NVS_H */

/* main.c — embed NvS in C via the C ABI (libnvs.so).
 *
 * Build & run (from the repo root):
 *
 *   go build -buildmode=c-shared -o examples/c_embed/libnvs.so ./cbridge
 *   cd examples/c_embed && gcc -o c_embed main.c -L. -lnvs -Wl,-rpath,'$ORIGIN' && ./c_embed
 *
 * Exercises: eval (define + run), call with args, error propagation,
 * and the nvs_free ownership contract.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "nvs.h"

static int failures = 0;

static void check(int cond, const char *label) {
	if (cond) {
		printf("ok: %s\n", label);
	} else {
		printf("FAIL: %s\n", label);
		failures++;
	}
}

int main(void) {
	/* 1. eval: define a function in the persistent environment. */
	char *r = nvs_eval("fn add(a, b) { a + b }\nlet greeting = \"hello from C\"");
	check(r != NULL, "eval returned non-null");
	printf("eval -> %s\n", r);
	nvs_free(r);

	/* 2. eval: run an expression and read the result envelope. */
	r = nvs_eval("6 * 7");
	check(r != NULL && strstr(r, "\"ok\":true") && strstr(r, "\"result\":42"),
	      "eval 6*7 -> 42");
	printf("eval -> %s\n", r);
	nvs_free(r);

	/* 3. call: invoke the NvS function defined above with JSON args. */
	r = nvs_call("add", "[20, 22]");
	check(r != NULL && strstr(r, "\"ok\":true") && strstr(r, "\"result\":42"),
	      "call add(20,22) -> 42");
	printf("call -> %s\n", r);
	nvs_free(r);

	/* 4. call: strings round-trip. */
	r = nvs_call("str", "[255]");
	check(r != NULL && strstr(r, "\"ok\":true") && strstr(r, "\"255\""),
	      "call str(255) -> \"255\"");
	printf("call -> %s\n", r);
	nvs_free(r);

	/* 5. error case: unknown function -> honest ok:false envelope. */
	r = nvs_call("no_such_function", "[]");
	check(r != NULL && strstr(r, "\"ok\":false"),
	      "call unknown fn -> ok:false");
	printf("call -> %s\n", r);
	nvs_free(r);

	/* 6. error case: NvS runtime error propagates. */
	r = nvs_eval("1 + ");
	check(r != NULL && strstr(r, "\"ok\":false"),
	      "eval bad syntax -> ok:false");
	printf("eval -> %s\n", r);
	nvs_free(r);

	if (failures == 0) {
		printf("ALL C EMBED CHECKS PASSED\n");
		return 0;
	}
	printf("%d CHECK(S) FAILED\n", failures);
	return 1;
}

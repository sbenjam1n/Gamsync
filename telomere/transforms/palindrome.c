/* palindrome.c — Append reversed pattern to create a palindromic loop */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

static void transform_palindrome(t_telomere *x, int argc, t_atom *argv) {
    (void)argc; (void)argv;

    int n = pattern_num_events(x);
    if (n == 0) return;

    /* Build palindrome: scale originals to first half, mirror to second */
    int new_size = n * 2;
    t_float buf[TELOMERE_MAX_EVENTS];
    int count;
    pattern_copy_to(x, buf, &count);

    pattern_resize(x, new_size);
    for (int i = 0; i < n; i++) {
        /* First half: compress to 0.0–0.5 */
        pattern_set_event(x, i, buf[i] * 0.5f);
        /* Second half: mirror into 0.5–1.0 */
        pattern_set_event(x, new_size - 1 - i, 0.5f + buf[i] * 0.5f);
    }

    pattern_sort(x);
}

void palindrome_register(void) {
    telomere_register_transform(
        gensym("palindrome"),
        transform_palindrome,
        "Append reversed pattern to create palindromic loop",
        0, 0
    );
}

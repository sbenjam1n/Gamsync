/* reverse.c — Reverse the temporal order of the pattern */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

static void transform_reverse(t_telomere *x, int argc, t_atom *argv) {
    (void)argc; (void)argv;

    int n = pattern_num_events(x);
    if (n == 0) return;

    /* Map event at position p to (1.0 - p) */
    for (int i = 0; i < n; i++) {
        t_float val = 1.0f - pattern_get_event(x, i);
        if (val < 0.0f) val = 0.0f;
        if (val >= 1.0f) val = 0.0f; /* wrap 1.0 to 0.0 */
        pattern_set_event(x, i, val);
    }

    pattern_sort(x);
}

void reverse_register(void) {
    telomere_register_transform(
        gensym("reverse"),
        transform_reverse,
        "Reverse the temporal order of the pattern",
        0, 0
    );
}

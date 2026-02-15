/* slow.c — Expand pattern by factor N (keep only 1/N of the cycle) */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

static void transform_slow(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    int factor = (int)atom_getfloatarg(0, argc, argv);
    if (factor < 2) factor = 2;

    /* Scale all positions into a narrower band: 0.0 – 1/factor */
    t_float scale = 1.0f / (t_float)factor;
    for (int i = 0; i < n; i++) {
        t_float val = pattern_get_event(x, i) * scale;
        pattern_set_event(x, i, val);
    }

    pattern_sort(x);
}

void slow_register(void) {
    telomere_register_transform(
        gensym("slow"),
        transform_slow,
        "Expand pattern by factor N (stretch into first 1/N of cycle)",
        1, 1
    );
}

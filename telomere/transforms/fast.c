/* fast.c — Compress pattern to repeat N times per cycle */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"

static void transform_fast(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    int factor = (int)atom_getfloatarg(0, argc, argv);
    if (factor < 2) factor = 2;

    /* Copy original pattern */
    t_float orig[TELOMERE_MAX_EVENTS];
    int count;
    pattern_copy_to(x, orig, &count);

    int new_size = count * factor;
    if (new_size > TELOMERE_MAX_EVENTS) new_size = TELOMERE_MAX_EVENTS;

    pattern_resize(x, new_size);

    t_float sub_len = 1.0f / (t_float)factor;
    int idx = 0;
    for (int rep = 0; rep < factor && idx < new_size; rep++) {
        t_float base = sub_len * (t_float)rep;
        for (int i = 0; i < count && idx < new_size; i++) {
            t_float val = base + orig[i] * sub_len;
            if (val >= 1.0f) val -= 1.0f;
            pattern_set_event(x, idx++, val);
        }
    }

    pattern_resize(x, idx);
    pattern_sort(x);
}

void fast_register(void) {
    telomere_register_transform(
        gensym("fast"),
        transform_fast,
        "Compress pattern to repeat N times per cycle",
        1, 1
    );
}

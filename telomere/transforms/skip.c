/* skip.c — Probabilistically remove events from the pattern */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"
#include <stdlib.h>

static void transform_skip(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    t_float prob = atom_getfloatarg(0, argc, argv);
    if (prob < 0.0f) prob = 0.0f;
    if (prob > 1.0f) prob = 1.0f;

    /* Copy surviving events into a temp buffer */
    t_float survivors[TELOMERE_MAX_EVENTS];
    int surv_count = 0;

    for (int i = 0; i < n; i++) {
        float r = (float)rand() / (float)RAND_MAX;
        if (r >= prob) {
            survivors[surv_count++] = pattern_get_event(x, i);
        }
    }

    pattern_replace(x, survivors, surv_count);
}

void skip_register(void) {
    telomere_register_transform(
        gensym("skip"),
        transform_skip,
        "Probabilistically remove events from the pattern",
        1, 1
    );
}

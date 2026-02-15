/* degrade.c — Probabilistically remove events (destructive version) */

#include "../telomere_transform.h"
#include "../telomere_pattern_api.h"
#include <stdlib.h>

static void transform_degrade(t_telomere *x, int argc, t_atom *argv) {
    int n = pattern_num_events(x);
    if (n == 0) return;

    t_float prob = atom_getfloatarg(0, argc, argv);
    if (prob < 0.0f) prob = 0.0f;
    if (prob > 1.0f) prob = 1.0f;

    /* Remove events with given probability, keeping at least 1 */
    t_float survivors[TELOMERE_MAX_EVENTS];
    int surv_count = 0;

    for (int i = 0; i < n; i++) {
        float r = (float)rand() / (float)RAND_MAX;
        if (r >= prob) {
            survivors[surv_count++] = pattern_get_event(x, i);
        }
    }

    /* Ensure at least one event survives */
    if (surv_count == 0 && n > 0) {
        survivors[0] = pattern_get_event(x, 0);
        surv_count = 1;
    }

    pattern_replace(x, survivors, surv_count);
}

void degrade_register(void) {
    telomere_register_transform(
        gensym("degrade"),
        transform_degrade,
        "Probabilistically remove events (keeps at least one)",
        1, 1
    );
}

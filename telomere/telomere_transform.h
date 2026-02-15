/* telomere_transform.h — Transform interface definition */
#ifndef TELOMERE_TRANSFORM_H
#define TELOMERE_TRANSFORM_H

#include "telomere.h"

/* Every transform function has this signature */
typedef void (*t_transform_fn)(t_telomere *x, int argc, t_atom *argv);

/* Registry entry — linked list node */
typedef struct _transform_entry {
    t_symbol               *name;
    t_transform_fn          fn;
    const char             *description;
    int                     min_args;
    int                     max_args;   /* -1 = unlimited */
    struct _transform_entry *next;
} t_transform_entry;

/* --- Registry API --- */
void              telomere_register_transform(t_symbol *name, t_transform_fn fn,
                      const char *description, int min_args, int max_args);
t_transform_entry *telomere_lookup_transform(t_symbol *name);
t_transform_entry *telomere_get_registry_head(void);
void              telomere_registry_free(void);

/* --- Builtins aggregator --- */
void telomere_transforms_builtins_setup(void);

#endif /* TELOMERE_TRANSFORM_H */

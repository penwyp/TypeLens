#include <dlfcn.h>
#include <stdio.h>
#include <stdlib.h>

typedef const char *(*get_device_id_fn)(const char *);

int main(int argc, char **argv) {
  const char *lib_path = argc > 1 ? argv[1] : "/Applications/Typeless.app/Contents/Resources/lib/util-helper/build/libUtilHelper.dylib";
  const char *app_name = argc > 2 ? argv[2] : "Typeless";

  void *handle = dlopen(lib_path, RTLD_NOW);
  if (!handle) {
    fprintf(stderr, "dlopen failed: %s\n", dlerror());
    return 2;
  }

  dlerror();
  get_device_id_fn get_device_id = (get_device_id_fn)dlsym(handle, "getDeviceId");
  const char *sym_err = dlerror();
  if (sym_err != NULL) {
    fprintf(stderr, "dlsym(getDeviceId) failed: %s\n", sym_err);
    dlclose(handle);
    return 3;
  }

  const char *value = get_device_id(app_name);
  if (value == NULL) {
    fprintf(stderr, "getDeviceId returned NULL\n");
    dlclose(handle);
    return 4;
  }

  printf("%s\n", value);
  dlclose(handle);
  return 0;
}

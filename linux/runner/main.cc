#include "my_application.h"
#include "single_instance_manager.h"

int main(int argc, char** argv) {
  const bool is_multi_window =
      argc > 1 && g_strcmp0(argv[1], "multi_window") == 0;
  SingleInstanceManager single_instance;
  if (!is_multi_window) {
    if (!single_instance.Initialize()) {
      return 1;
    }
    if (!single_instance.IsPrimaryInstance()) {
      single_instance.NotifyPrimaryInstance();
      return 0;
    }
  }

  g_autoptr(MyApplication) app = my_application_new();
  if (!is_multi_window) {
    single_instance.StartListening([app]() { my_application_present_main_window(app); });
  }

  const int exit_code = g_application_run(G_APPLICATION(app), argc, argv);
  if (!is_multi_window) {
    single_instance.Shutdown();
  }
  return exit_code;
}

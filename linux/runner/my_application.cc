#include "my_application.h"

#include <flutter_linux/flutter_linux.h>
#ifdef GDK_WINDOWING_X11
#include <gdk/gdkx.h>
#endif

#include <desktop_multi_window/desktop_multi_window_plugin.h>
#include <screen_retriever_linux/screen_retriever_linux_plugin.h>
#include <url_launcher_linux/url_launcher_plugin.h>
#include <window_manager/window_manager_plugin.h>

#include "flutter/generated_plugin_registrant.h"

struct _MyApplication {
  GtkApplication parent_instance;
  char** dart_entrypoint_arguments;
};

G_DEFINE_TYPE(MyApplication, my_application, GTK_TYPE_APPLICATION)

// 在 GTK 应用激活后设置默认窗口图标，避免在进程早期访问未就绪的
// screen/icon-theme 导致 GTK 断言告警。
static void apply_default_window_icon(void) {
  GtkIconTheme* theme = gtk_icon_theme_get_default();
  if (GTK_IS_ICON_THEME(theme) &&
      gtk_icon_theme_has_icon(theme, "clawdsecbot")) {
    gtk_window_set_default_icon_name("clawdsecbot");
    return;
  }

  if (g_file_test("scripts/icon_1024.png", G_FILE_TEST_EXISTS)) {
    gtk_window_set_default_icon_from_file("scripts/icon_1024.png", NULL);
  }
}

// Called when first Flutter frame received.
static void first_frame_cb(MyApplication* self, FlView* view) {
  GtkWidget* widget = GTK_WIDGET(view);
  if (!GTK_IS_WIDGET(widget)) return;
  gtk_widget_show(gtk_widget_get_toplevel(widget));
}

// Safety-net handler for window close: hide the window instead of destroying
// it so the Flutter engine and its implicit FlView stay alive.  Used for
// both the main window and dynamically-created sub-windows.
//
// Connected AFTER fl_register_plugins / register_plugins_for_sub_window so
// that window_manager's own delete-event handler gets first priority.
// The delete-event signal uses g_signal_accumulator_true_handled:
//   - If window_manager returns TRUE (setPreventClose works) our handler
//     never runs — the Dart-level onWindowClose takes over.
//   - If window_manager returns FALSE or is not registered, this handler
//     catches the event as a last resort, preventing gtk_widget_destroy
//     and the resulting gtk_widget_get_scale_factor assertion failure.
static gboolean on_sub_window_delete_event(GtkWidget* widget,
                                           GdkEvent* event,
                                           gpointer user_data) {
  gtk_widget_hide(widget);
  return TRUE;
}

// Apply a dark style to GTK header bars so they match the app's custom dark
// theme on every Linux desktop. The focused (active) state uses a slightly
// deeper colour than the unfocused (backdrop) state, giving a subtle visual
// cue without flashing white or making the window-control buttons invisible.
//
// KEY: we set both background-color AND background-image:none. Many GTK
// themes (Ubuntu Yaru, Adwaita, etc.) paint headerbars via background-image
// with a linear-gradient, which overlays background-color. Without clearing
// background-image, the theme gradient covers our colour on focus change.
//
// Priority: GTK_STYLE_PROVIDER_PRIORITY_USER (800) — the highest available
// level — guarantees our rules override any theme-supplied headerbar styles.
static void apply_dark_headerbar_css(void) {
  const gchar* css =
      // Focused (active) window — slightly deeper than backdrop
      "headerbar {"
      "  background-color: rgba(10, 10, 24, 1.0);"
      "  background-image: none;"
      "  border-bottom: 1px solid rgba(255, 255, 255, 0.1);"
      "  box-shadow: none;"
      "}"
      // Unfocused (backdrop) window — matches main_page's Color(0xFF1A1A2E)
      "headerbar:backdrop {"
      "  background-color: rgba(26, 26, 46, 1.0);"
      "  background-image: none;"
      "  border-bottom: 1px solid rgba(255, 255, 255, 0.06);"
      "  box-shadow: none;"
      "}"
      // Title text
      "headerbar .title {"
      "  color: rgba(255, 255, 255, 0.92);"
      "  font-weight: bold;"
      "}"
      "headerbar:backdrop .title {"
      "  color: rgba(255, 255, 255, 0.65);"
      "}"
      "headerbar .subtitle {"
      "  color: rgba(255, 255, 255, 0.5);"
      "}"
      "headerbar:backdrop .subtitle {"
      "  color: rgba(255, 255, 255, 0.35);"
      "}"
      // Window-control & toolbar buttons
      "headerbar button {"
      "  color: rgba(255, 255, 255, 0.75);"
      "  background: transparent;"
      "  background-image: none;"
      "  border: none;"
      "  box-shadow: none;"
      "  outline: none;"
      "}"
      "headerbar:backdrop button {"
      "  color: rgba(255, 255, 255, 0.45);"
      "  background-image: none;"
      "}"
      "headerbar button:hover {"
      "  background: rgba(255, 255, 255, 0.14);"
      "  background-image: none;"
      "  color: rgba(255, 255, 255, 1.0);"
      "}"
      "headerbar button:active {"
      "  background: rgba(255, 255, 255, 0.22);"
      "  background-image: none;"
      "  color: rgba(255, 255, 255, 1.0);"
      "}";

  GtkCssProvider* provider = gtk_css_provider_new();
  gtk_css_provider_load_from_data(provider, css, -1, NULL);
  gtk_style_context_add_provider_for_screen(
      gdk_screen_get_default(), GTK_STYLE_PROVIDER(provider),
      GTK_STYLE_PROVIDER_PRIORITY_USER);
  g_object_unref(provider);
}

// Register only the plugins that sub-windows need.
// desktop_multi_window is already handled internally by the plugin itself.
// tray_manager is main-window only (system tray icon).
static void register_plugins_for_sub_window(FlPluginRegistry* registry) {
  g_autoptr(FlPluginRegistrar) screen_retriever_registrar =
      fl_plugin_registry_get_registrar_for_plugin(
          registry, "ScreenRetrieverLinuxPlugin");
  screen_retriever_linux_plugin_register_with_registrar(
      screen_retriever_registrar);

  g_autoptr(FlPluginRegistrar) url_launcher_registrar =
      fl_plugin_registry_get_registrar_for_plugin(registry, "UrlLauncherPlugin");
  url_launcher_plugin_register_with_registrar(url_launcher_registrar);

  g_autoptr(FlPluginRegistrar) window_manager_registrar =
      fl_plugin_registry_get_registrar_for_plugin(
          registry, "WindowManagerPlugin");
  window_manager_plugin_register_with_registrar(window_manager_registrar);

  // Safety-net: hide dynamically created sub-windows instead of destroying
  // them when the close button is clicked and window_manager didn't intercept.
  GtkWidget* view_widget = GTK_WIDGET(FL_VIEW(registry));
  GtkWidget* toplevel = gtk_widget_get_toplevel(view_widget);
  if (GTK_IS_WINDOW(toplevel)) {
    gtk_window_set_title(GTK_WINDOW(toplevel), "ClawdSecbot");
    g_signal_connect(toplevel, "delete-event",
                     G_CALLBACK(on_sub_window_delete_event), NULL);
  }
}

// Implements GApplication::activate.
static void my_application_activate(GApplication* application) {
  MyApplication* self = MY_APPLICATION(application);

  GList* windows = gtk_application_get_windows(GTK_APPLICATION(application));
  if (windows != nullptr) {
    GtkWindow* existing_window = GTK_WINDOW(windows->data);
    gtk_widget_show(GTK_WIDGET(existing_window));
    gtk_window_present(existing_window);
    return;
  }

  apply_default_window_icon();
  apply_dark_headerbar_css();

  // Prevent GTK from auto-quitting when a window is closed. App lifecycle is
  // managed by the Flutter layer.
  g_application_hold(application);

  GtkWindow* window =
      GTK_WINDOW(gtk_application_window_new(GTK_APPLICATION(application)));

  // Use a header bar when running in GNOME as this is the common style used
  // by applications and is the setup most users will be using (e.g. Ubuntu
  // desktop).
  // If running on X and not using GNOME then just use a traditional title bar
  // in case the window manager does more exotic layout, e.g. tiling.
  // If running on Wayland assume the header bar will work (may need changing
  // if future cases occur).
  gboolean use_header_bar = TRUE;
#ifdef GDK_WINDOWING_X11
  GdkScreen* screen = gtk_window_get_screen(window);
  if (GDK_IS_X11_SCREEN(screen)) {
    const gchar* wm_name = gdk_x11_screen_get_window_manager_name(screen);
    if (g_strcmp0(wm_name, "GNOME Shell") != 0) {
      use_header_bar = FALSE;
    }
  }
#endif
  if (use_header_bar) {
    GtkHeaderBar* header_bar = GTK_HEADER_BAR(gtk_header_bar_new());
    gtk_widget_show(GTK_WIDGET(header_bar));
    gtk_header_bar_set_title(header_bar, "ClawdSecbot");
    gtk_header_bar_set_show_close_button(header_bar, TRUE);
    gtk_window_set_titlebar(window, GTK_WIDGET(header_bar));
  } else {
    gtk_window_set_title(window, "ClawdSecbot");
  }

  gtk_window_set_default_size(window, 600, 780);

  g_autoptr(FlDartProject) project = fl_dart_project_new();
  fl_dart_project_set_dart_entrypoint_arguments(
      project, self->dart_entrypoint_arguments);

  FlView* view = fl_view_new(project);
  GdkRGBA background_color;
  gdk_rgba_parse(&background_color, "#0f0f23");
  fl_view_set_background_color(view, &background_color);
  gtk_widget_show(GTK_WIDGET(view));
  gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(view));

  g_signal_connect_swapped(view, "first-frame", G_CALLBACK(first_frame_cb),
                           self);
  gtk_widget_realize(GTK_WIDGET(view));

  fl_register_plugins(FL_PLUGIN_REGISTRY(view));

  // Register plugins on sub-windows created by desktop_multi_window.
  // Skip desktop_multi_window (handled internally) and tray_manager (main only).
  desktop_multi_window_plugin_set_window_created_callback(
      register_plugins_for_sub_window);

  // Safety-net for ALL windows (main + sub-window processes): hide instead of
  // destroy when the close button is clicked and window_manager's
  // setPreventClose didn't intercept.  Connected AFTER fl_register_plugins so
  // window_manager's handler gets first priority.
  g_signal_connect(window, "delete-event",
                   G_CALLBACK(on_sub_window_delete_event), NULL);

  gtk_widget_grab_focus(GTK_WIDGET(view));
}

// Implements GApplication::local_command_line.
static gboolean my_application_local_command_line(GApplication* application,
                                                  gchar*** arguments,
                                                  int* exit_status) {
  MyApplication* self = MY_APPLICATION(application);
  // Strip out the first argument as it is the binary name.
  self->dart_entrypoint_arguments = g_strdupv(*arguments + 1);

  g_autoptr(GError) error = nullptr;
  if (!g_application_register(application, nullptr, &error)) {
    g_warning("Failed to register: %s", error->message);
    *exit_status = 1;
    return TRUE;
  }

  g_application_activate(application);
  *exit_status = 0;

  return TRUE;
}

// Implements GApplication::startup.
static void my_application_startup(GApplication* application) {
  G_APPLICATION_CLASS(my_application_parent_class)->startup(application);
}

// Implements GApplication::shutdown.
static void my_application_shutdown(GApplication* application) {
  G_APPLICATION_CLASS(my_application_parent_class)->shutdown(application);
}

// Implements GObject::dispose.
static void my_application_dispose(GObject* object) {
  MyApplication* self = MY_APPLICATION(object);
  g_clear_pointer(&self->dart_entrypoint_arguments, g_strfreev);
  G_OBJECT_CLASS(my_application_parent_class)->dispose(object);
}

static void my_application_class_init(MyApplicationClass* klass) {
  G_APPLICATION_CLASS(klass)->activate = my_application_activate;
  G_APPLICATION_CLASS(klass)->local_command_line =
      my_application_local_command_line;
  G_APPLICATION_CLASS(klass)->startup = my_application_startup;
  G_APPLICATION_CLASS(klass)->shutdown = my_application_shutdown;
  G_OBJECT_CLASS(klass)->dispose = my_application_dispose;
}

static void my_application_init(MyApplication* self) {}

MyApplication* my_application_new() {
  g_set_prgname(APPLICATION_ID);
  g_set_application_name("ClawdSecbot");

  return MY_APPLICATION(g_object_new(my_application_get_type(),
                                     "application-id", APPLICATION_ID, "flags",
                                     G_APPLICATION_NON_UNIQUE, nullptr));
}

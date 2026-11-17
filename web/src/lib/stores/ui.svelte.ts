/** Shell UI state: sidebar rail and command palette. */
export class UiState {
  sidebarCollapsed = $state(false);
  paletteOpen = $state(false);

  toggleSidebar() {
    this.sidebarCollapsed = !this.sidebarCollapsed;
  }
  openPalette() {
    this.paletteOpen = true;
  }
}

export const ui = new UiState();

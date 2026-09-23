/*
 * Die Stellen, an denen ein aufbauendes Programm in die Oberfläche eingreift.
 *
 * Diese Oberfläche gehört zu einem Server mit genau einem Spiel. Wer mehrere
 * führen will, braucht eine Ebene darüber — Konten, eine Spieleübersicht, eine
 * zweite Art sich anzumelden. Das liegt woanders (operation-x-server) und
 * hängt sich hier ein, statt dass dieses Verzeichnis davon wüsste.
 *
 * Drei Steckplätze, mehr nicht. Jeder nimmt eine Svelte-Komponente auf; ist er
 * leer, zeigt die Oberfläche an dieser Stelle nichts.
 */
export const aufsatz = $state({
  /**
   * Was ein angemeldeter Zugang sieht, der kein Team ist.
   *
   * Die Oberfläche zeigt sonst das Spiel des angemeldeten Teams. Ein Zugang
   * ohne Team hätte nichts zu sehen — hier kommt hin, was er stattdessen
   * bekommt.
   */
  hauptansicht: null,

  /** Ein weiterer Block auf der Anmeldeseite, unter der Anmeldung. */
  anmeldung: null,

  /** Ein Knopf in der Kopfzeile, neben "Abmelden". */
  kopfzeile: null,
})

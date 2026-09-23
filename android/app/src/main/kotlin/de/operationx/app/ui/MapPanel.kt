package de.operationx.app.ui

import android.graphics.Color as AndroidColor
import android.view.ViewGroup
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import de.operationx.app.data.FieldMap
import de.operationx.app.data.LiveState
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.camera.CameraUpdateFactory
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.geometry.LatLngBounds
import org.maplibre.android.maps.MapView
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.Style
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.Point
import org.maplibre.geojson.Polygon
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Rect
import android.graphics.Typeface
import org.maplibre.android.style.expressions.Expression
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.FillLayer
import org.maplibre.android.style.layers.LineLayer
import org.maplibre.android.style.layers.PropertyFactory
import org.maplibre.android.style.layers.SymbolLayer
import org.maplibre.android.style.sources.GeoJsonSource
import kotlin.math.cos

/**
 * Die Lagekarte.
 *
 * Sie zeigt dasselbe wie im Browser und folgt denselben Regeln: Die Karte ist
 * der Untergrund und bleibt dunkel, damit Sektoren und Teams darüber lesbar
 * sind. Unschärfekreise werden als echte Flächen gezeichnet – ein Kreis mit
 * fester Pixelgröße verlöre beim Zoomen seine Bedeutung, hier stehen 200 Meter
 * für 200 Meter, auf jeder Stufe.
 */
@Composable
fun MapPanel(
    karte: MapHolder,
    field: FieldMap?,
    live: LiveState?,
    modifier: Modifier = Modifier,
) {
    val view = karte.view ?: return

    Box(modifier) {
    AndroidView(
        modifier = Modifier,
        // Immer dieselbe Karte. Sie gehört nicht dieser Ansicht, sondern dem
        // Feldbildschirm darüber – siehe rememberKarte().
        factory = { view },
        update = { karte.render(field, live) },
        onRelease = { child ->
            // Beim Verlassen nur aushängen, nicht zerstören: Compose baut beim
            // nächsten Mal einen neuen Rahmen, und eine View mit altem Elter
            // lässt sich dort nicht einhängen.
            (child.parent as? ViewGroup)?.removeView(child)
        },
    )

    // Die Kartendaten stammen von OpenStreetMap und stehen unter der ODbL.
    // Sie zu nennen ist keine Höflichkeit, sondern Bedingung. MapLibre legt
    // den Hinweis sonst hinter das ⓘ – eine Zeile, die niemand antippt, ist
    // für eine Namensnennung zu wenig.
    Text(
        "© OpenStreetMap-Mitwirkende",
        color = Color.White.copy(alpha = 0.85f),
        fontSize = 9.sp,
        modifier = Modifier
            .align(Alignment.BottomEnd)
            .padding(4.dp)
            .background(Color.Black.copy(alpha = 0.55f))
            .padding(horizontal = 4.dp, vertical = 1.dp),
    )
    }

    LaunchedEffect(field, live) { karte.render(field, live) }
}

/**
 * Die Karte für die Dauer eines Feldbildschirms.
 *
 * Sie lag vorher in der Lageansicht selbst – und damit im Tabwechsel: Wer auf
 * "Mission" ging und zurückkam, bekam eine neue Karte, eine schwarze Sekunde,
 * frisch geholte Kacheln und den Startausschnitt statt seines eigenen. Bei
 * einem Spiel, in dem man dutzende Male zwischen Lage und Mission wechselt,
 * ist das jedes Mal Zeit, Daten und Akku.
 *
 * Deshalb wird sie hier gehalten, eine Ebene über den Reitern, und dort unten
 * nur noch eingehängt. Der Lebenszyklus folgt jetzt dem der App und nicht mehr
 * dem des Reiters: Im Hintergrund hört das Rendern auf, beim Zurückkommen läuft
 * es weiter – mit demselben Ausschnitt, auf den zuletzt jemand geschaut hat.
 */
@Composable
fun rememberKarte(serverUrl: String): MapHolder {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    val karte = remember(serverUrl) { MapHolder() }

    remember(serverUrl) {
        MapLibre.getInstance(context)
        MapView(context).apply {
            onCreate(null)
            karte.view = this
            getMapAsync { map ->
                karte.map = map
                map.cameraPosition = CameraPosition.Builder()
                    .target(LatLng(51.0504, 13.7373)) // Dresden
                    .zoom(11.5)
                    .build()
                map.setStyle(Style.Builder().fromJson(darkRasterStyle(serverUrl))) { style ->
                    karte.style = style
                    karte.prepareLayers(style)
                    karte.nachzeichnen()
                }
            }
        }
    }

    DisposableEffect(lifecycleOwner, karte) {
        val beobachter = LifecycleEventObserver { _, ereignis ->
            val view = karte.view ?: return@LifecycleEventObserver
            when (ereignis) {
                Lifecycle.Event.ON_START -> view.onStart()
                Lifecycle.Event.ON_RESUME -> view.onResume()
                Lifecycle.Event.ON_PAUSE -> view.onPause()
                Lifecycle.Event.ON_STOP -> view.onStop()
                Lifecycle.Event.ON_DESTROY -> view.onDestroy()
                else -> Unit
            }
        }
        lifecycleOwner.lifecycle.addObserver(beobachter)

        onDispose {
            lifecycleOwner.lifecycle.removeObserver(beobachter)
            karte.view?.let {
                (it.parent as? ViewGroup)?.removeView(it)
                it.onPause()
                it.onStop()
                it.onDestroy()
            }
            karte.view = null
        }
    }

    return karte
}

/** Hält Karte und Quellen zusammen, damit update() nichts neu aufbauen muss. */
class MapHolder {
    var view: MapView? = null
    var map: MapLibreMap? = null
    var style: Style? = null

    /*
     * Was zuletzt zu zeichnen war.
     *
     * Der Stil ist erst nach ein paar hundert Millisekunden fertig, und bis
     * dahin verwirft render() jeden Aufruf. Vorher rief der Stil selbst am
     * Ende seiner Vorbereitung render() mit den Daten auf, die er von außen
     * mitbekommen hatte – das geht nicht mehr, seit die Karte eine Ebene
     * höher entsteht als die Ansicht, die die Daten hat. Also merkt sie sich
     * den letzten Stand und zeichnet ihn nach, sobald sie kann. Ohne das
     * bliebe die Karte leer, bis sich zufällig das nächste Mal etwas an der
     * Lage ändert.
     */
    private var letztesFeld: FieldMap? = null
    private var letzteLive: LiveState? = null

    private var sectorSource: GeoJsonSource? = null
    private var hotspotSource: GeoJsonSource? = null
    private var teamSource: GeoJsonSource? = null
    private var blurSource: GeoJsonSource? = null
    private var finaleSource: GeoJsonSource? = null

    fun prepareLayers(style: Style) {
        sectorSource = GeoJsonSource("sectors").also { style.addSource(it) }
        blurSource = GeoJsonSource("blur").also { style.addSource(it) }
        finaleSource = GeoJsonSource("finale").also { style.addSource(it) }
        hotspotSource = GeoJsonSource("hotspots").also { style.addSource(it) }
        teamSource = GeoJsonSource("teams").also { style.addSource(it) }

        style.addLayer(
            FillLayer("sector-fill", "sectors").withProperties(
                PropertyFactory.fillColor(AndroidColor.parseColor("#4F8EA3")),
                PropertyFactory.fillOpacity(0.16f),
            )
        )
        style.addLayer(
            LineLayer("sector-line", "sectors").withProperties(
                PropertyFactory.lineColor(AndroidColor.parseColor("#4F8EA3")),
                PropertyFactory.lineWidth(1.2f),
            )
        )
        style.addLayer(
            FillLayer("finale-fill", "finale").withProperties(
                PropertyFactory.fillColor(AndroidColor.parseColor("#D9A441")),
                PropertyFactory.fillOpacity(0.10f),
            )
        )
        style.addLayer(
            LineLayer("finale-line", "finale").withProperties(
                PropertyFactory.lineColor(AndroidColor.parseColor("#D9A441")),
                PropertyFactory.lineWidth(2f),
            )
        )
        style.addLayer(
            FillLayer("blur-fill", "blur").withProperties(
                PropertyFactory.fillColor(AndroidColor.parseColor("#35C0D6")),
                PropertyFactory.fillOpacity(0.14f),
            )
        )
        style.addLayer(
            CircleLayer("hotspot-dot", "hotspots").withProperties(
                PropertyFactory.circleRadius(6f),
                PropertyFactory.circleColor(AndroidColor.parseColor("#0B1215")),
                PropertyFactory.circleStrokeColor(AndroidColor.parseColor("#D9A441")),
                PropertyFactory.circleStrokeWidth(1.6f),
            )
        )
        // Beschriftungen als Bilder, nicht als Text.
        //
        // MapLibre kann Schrift nur zeichnen, wenn der Stil eine Quelle für
        // Schriftzeichen nennt ("glyphs"). Ein reiner Rasterstil hat keine, und
        // ein Server dafür wäre genau die Netzabhängigkeit, die dieses Spiel
        // sich nicht leisten kann. Deshalb wird jede Beschriftung einmal in ein
        // kleines Bild gezeichnet und als Symbol eingehängt.
        style.addLayer(
            SymbolLayer("hotspot-label", "hotspots").withProperties(
                PropertyFactory.iconImage(Expression.get("icon")),
                PropertyFactory.iconAllowOverlap(true),
                PropertyFactory.iconOffset(arrayOf(0f, -14f)),
            )
        )
        style.addLayer(
            CircleLayer("team-dot", "teams").withProperties(
                PropertyFactory.circleRadius(7f),
                PropertyFactory.circleColor(Expression.toColor(Expression.get("color"))),
                // Der eigene Punkt trägt einen hellen Ring, alle anderen einen
                // dunklen. Damit beantwortet die Karte die Frage "welcher bin
                // ich?" ohne Lesen — im Gehen, bei Sonne.
                PropertyFactory.circleStrokeColor(Expression.toColor(Expression.get("ring"))),
                PropertyFactory.circleStrokeWidth(
                    Expression.switchCase(
                        Expression.eq(Expression.get("mine"), Expression.literal(true)),
                        Expression.literal(3f),
                        Expression.literal(2f),
                    )
                ),
            )
        )
        style.addLayer(
            SymbolLayer("team-label", "teams").withProperties(
                PropertyFactory.iconImage(Expression.get("icon")),
                PropertyFactory.iconAllowOverlap(true),
                PropertyFactory.iconOffset(arrayOf(0f, 20f)),
            )
        )
    }

    /**
     * Ob die Karte schon einmal auf das Spielgebiet ausgerichtet wurde.
     *
     * Die Karte stand fest auf Dresden, Zoomstufe 11,5 – auch dann, wenn das
     * Spiel woanders läuft, und auch dann, wenn der eigene Punkt am Rand liegt.
     * Wer wissen wollte, wo er steht, musste erst schieben und ziehen. Einmal
     * ausrichten genügt: Danach gehört die Karte dem Spieler, und ein
     * Nachführen bei jeder Meldung würde ihm die Ansicht unter den Fingern
     * wegziehen.
     */
    private var framed = false

    /** Zeichnet den zuletzt bekannten Stand – für den Moment, in dem der Stil fertig wird. */
    fun nachzeichnen() = render(letztesFeld, letzteLive)

    fun render(field: FieldMap?, live: LiveState?) {
        letztesFeld = field
        letzteLive = live

        val style = style ?: return

        field?.let { f ->
            // Die Sektoren wurden bisher nie an die Karte gegeben: Die Quelle
            // war angelegt, die Ebenen waren da, nur befüllt hat sie niemand.
            sectorSource?.setGeoJson(
                FeatureCollection.fromFeatures(
                    f.sectors.mapNotNull { sec ->
                        val raw = sec.geometry?.toString() ?: return@mapNotNull null
                        runCatching {
                            Feature.fromGeometry(Polygon.fromJson(raw))
                        }.getOrNull()
                    }
                )
            )

            hotspotSource?.setGeoJson(
                FeatureCollection.fromFeatures(
                    f.hotspots.map { h ->
                        val label = h.number.toString().padStart(2, '0')
                        Feature.fromGeometry(Point.fromLngLat(h.lng, h.lat)).apply {
                            addStringProperty("icon", labelImage(style, label, "#D9A441"))
                        }
                    }
                )
            )
        }

        live?.let { l ->
            teamSource?.setGeoJson(
                FeatureCollection.fromFeatures(
                    l.positions.map { p ->
                        Feature.fromGeometry(Point.fromLngLat(p.lng, p.lat)).apply {
                            val name = p.display.ifBlank { p.callsign }
                            addStringProperty("icon", labelImage(style, name, "#EEF4F6"))
                            addStringProperty("color", p.color.ifBlank { "#35C0D6" })
                            addStringProperty("ring", if (p.self) "#EEF4F6" else "#0B1215")
                            addBooleanProperty("mine", p.self)
                        }
                    }
                )
            )

            blurSource?.setGeoJson(
                FeatureCollection.fromFeatures(
                    l.positions.filter { it.blurM > 0 }.map { p ->
                        Feature.fromGeometry(circle(p.lat, p.lng, p.blurM))
                    }
                )
            )

            frameOnce(l, field)

            val finale = l.finale
            finaleSource?.setGeoJson(
                if (finale != null && finale.active) {
                    FeatureCollection.fromFeature(
                        Feature.fromGeometry(circle(finale.lat, finale.lng, finale.radiusM))
                    )
                } else {
                    FeatureCollection.fromFeatures(emptyList())
                }
            )
        }
    }

    /** Richtet die Karte einmalig aus: auf den eigenen Punkt, sonst aufs Feld. */
    private fun frameOnce(live: LiveState, field: FieldMap?) {
        if (framed) return
        val map = map ?: return

        val self = live.positions.firstOrNull { it.self }
        if (self != null) {
            map.cameraPosition = CameraPosition.Builder()
                .target(LatLng(self.lat, self.lng))
                .zoom(14.0)
                .build()
            framed = true
            return
        }

        val punkte = field?.hotspots.orEmpty()
        if (punkte.size >= 2) {
            val bounds = LatLngBounds.Builder()
                .also { b -> punkte.forEach { b.include(LatLng(it.lat, it.lng)) } }
                .build()
            runCatching {
                map.easeCamera(CameraUpdateFactory.newLatLngBounds(bounds, 80), 400)
                framed = true
            }
        }
    }

    /**
     * Eine Beschriftung als Bild.
     *
     * MapLibre zeichnet Schrift nur mit einer Schriftzeichenquelle im Stil, und
     * ein Rasterstil hat keine. Ein Server dafür wäre genau die
     * Netzabhängigkeit, die dieses Spiel sich nicht leisten kann – im Funkloch
     * verschwänden sonst alle Beschriftungen.
     *
     * Also wird jede einmal gezeichnet und unter ihrem Text abgelegt. Bei zwanzig
     * Hotspots und einer Handvoll Teams sind das ein paar Dutzend winzige
     * Bilder; sie entstehen einmal und bleiben.
     */
    private val labelIds = mutableSetOf<String>()

    fun labelImage(style: Style, text: String, color: String): String {
        val id = "label-$color-$text"
        if (labelIds.add(id)) {
            style.addImage(id, drawLabel(text, AndroidColor.parseColor(color)))
        }
        return id
    }

    private fun drawLabel(text: String, color: Int): Bitmap {
        val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            this.color = color
            textSize = 30f
            typeface = Typeface.create(Typeface.DEFAULT, Typeface.BOLD)
            textAlign = Paint.Align.CENTER
        }
        // Ein dunkler Umriss, damit die Schrift über hellen Kartenflächen
        // lesbar bleibt – dieselbe Aufgabe, die sonst der Halo übernimmt.
        val halo = Paint(paint).apply {
            this.color = AndroidColor.parseColor("#0B1215")
            style = Paint.Style.STROKE
            strokeWidth = 5f
        }

        val bounds = Rect()
        paint.getTextBounds(text, 0, text.length, bounds)
        val w = (bounds.width() + 16).coerceAtLeast(24)
        val h = (bounds.height() + 14).coerceAtLeast(24)

        val bitmap = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
        val canvas = Canvas(bitmap)
        val x = w / 2f
        val y = h / 2f - (paint.descent() + paint.ascent()) / 2f
        canvas.drawText(text, x, y, halo)
        canvas.drawText(text, x, y, paint)
        return bitmap
    }

    /** Ein Kreis mit Radius in Metern, als Polygon aus 64 Punkten. */
    private fun circle(lat: Double, lng: Double, radiusM: Double, steps: Int = 64): Polygon {
        val earth = 6371008.8
        val dLat = Math.toDegrees(radiusM / earth)
        val dLng = dLat / cos(Math.toRadians(lat))

        val ring = (0..steps).map { i ->
            val a = 2 * Math.PI * i / steps
            Point.fromLngLat(lng + dLng * kotlin.math.cos(a), lat + dLat * kotlin.math.sin(a))
        }
        return Polygon.fromLngLats(listOf(ring))
    }
}

/**
 * Kartenstil.
 *
 * Die Kacheln kommen vom Spielserver, nicht direkt von OpenStreetMap. Der
 * Server holt jede einmal und legt sie ab – acht Telefone laden damit nicht
 * achtmal dasselbe über Mobilfunk, und was einmal jemand angesehen hat, ist
 * auch im Funkloch noch da.
 *
 * Vorab geladen wird nichts; das untersagen die Bedingungen von OpenStreetMap.
 * Wer ganz ohne Netz spielen will, legt einen eigenen Kachelsatz neben das
 * Programm.
 */
private fun darkRasterStyle(serverUrl: String): String = """
{
  "version": 8,
  "sources": {
    "osm": {
      "type": "raster",
      "tiles": ["$serverUrl/api/opx/tiles/{z}/{x}/{y}"],
      "tileSize": 256,
      "attribution": "© OpenStreetMap-Mitwirkende"
    }
  },
  "layers": [
    { "id": "ground", "type": "background", "paint": { "background-color": "#0B1215" } },
    {
      "id": "osm",
      "type": "raster",
      "source": "osm",
      "paint": {
        "raster-brightness-min": 0,
        "raster-brightness-max": 0.28,
        "raster-saturation": -0.75,
        "raster-contrast": 0.2,
        "raster-opacity": 0.85
      }
    }
  ]
}
""".trimIndent()

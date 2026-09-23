import SwiftUI
import MapKit
import UIKit

/// Die Lagekarte.
///
/// MapKit, aber nicht Apples Karte: Der Untergrund kommt als Kachelauflage vom
/// Spielserver, derselbe wie im Browser und in der Android-App. Der Server holt
/// jede Kachel einmal von OpenStreetMap und liefert sie an alle Geräte – das
/// spart Mobilfunk, hält die Karte im Funkloch am Leben und ist der Grund,
/// warum hier keine fremde Kartenbibliothek nötig ist.
///
/// Unschärfekreise sind echte Kreise in Metern, keine Punkte fester Größe:
/// 200 Meter bleiben 200 Meter, auf jeder Zoomstufe. Alles andere würde die
/// Regel beim Hineinzoomen aufweichen.
struct MapPanel: UIViewRepresentable {

    let serverUrl: String
    let feld: FieldMap?
    let live: LiveState?

    func makeCoordinator() -> Koordinator { Koordinator() }

    func makeUIView(context: Context) -> MKMapView {
        let karte = MKMapView()
        karte.delegate = context.coordinator
        karte.pointOfInterestFilter = .excludingAll
        karte.showsCompass = false
        karte.showsUserLocation = true

        if !serverUrl.isEmpty {
            let auflage = MKTileOverlay(urlTemplate: serverUrl + "/api/opx/tiles/{z}/{x}/{y}")
            // Ersetzt Apples Grundkarte vollständig – sonst läge unsere Karte
            // auf einer zweiten, und die Beschriftungen stünden doppelt.
            auflage.canReplaceMapContent = true
            auflage.maximumZ = 19
            karte.addOverlay(auflage, level: .aboveLabels)
            context.coordinator.kachelAuflage = auflage
        }

        // Dresden, bis das erste Spielfeld da ist.
        karte.setRegion(MKCoordinateRegion(
            center: CLLocationCoordinate2D(latitude: 51.0504, longitude: 13.7373),
            span: MKCoordinateSpan(latitudeDelta: 0.12, longitudeDelta: 0.12)
        ), animated: false)

        return karte
    }

    func updateUIView(_ karte: MKMapView, context: Context) {
        context.coordinator.zeichnen(karte, feld: feld, live: live)
    }

    final class Koordinator: NSObject, MKMapViewDelegate {
        var kachelAuflage: MKTileOverlay?
        private var gezeigtesFeld: String?
        private var ausschnittGesetzt = false

        func zeichnen(_ karte: MKMapView, feld: FieldMap?, live: LiveState?) {
            // Sektoren und Hotspots ändern sich während eines Spiels nicht.
            // Sie jedes Mal neu zu zeichnen ließe die Karte flackern.
            if let feld, gezeigtesFeld != feld.game {
                gezeigtesFeld = feld.game
                sektorenZeichnen(karte, feld)
            }

            teamsZeichnen(karte, live)

            if !ausschnittGesetzt, let feld, let rahmen = rahmen(um: feld) {
                ausschnittGesetzt = true
                karte.setVisibleMapRect(rahmen,
                                        edgePadding: UIEdgeInsets(top: 40, left: 40,
                                                                  bottom: 40, right: 40),
                                        animated: false)
            }
        }

        private func sektorenZeichnen(_ karte: MKMapView, _ feld: FieldMap) {
            let alt = karte.overlays.filter { !($0 is MKTileOverlay) }
            karte.removeOverlays(alt)

            for sektor in feld.sectors ?? [] {
                for ring in sektor.geometry?.rings ?? [] {
                    let punkte = ring.compactMap { paar -> CLLocationCoordinate2D? in
                        guard paar.count >= 2 else { return nil }
                        // GeoJSON zählt Längengrad zuerst.
                        return CLLocationCoordinate2D(latitude: paar[1], longitude: paar[0])
                    }
                    guard punkte.count >= 3 else { continue }
                    let flaeche = MKPolygon(coordinates: punkte, count: punkte.count)
                    flaeche.title = sektor.code
                    karte.addOverlay(flaeche, level: .aboveLabels)
                }
            }

            let alteMarken = karte.annotations.filter { $0 is HotspotMarke }
            karte.removeAnnotations(alteMarken)

            for h in feld.hotspots ?? [] {
                guard let lat = h.lat, let lng = h.lng else { continue }
                let marke = HotspotMarke()
                marke.coordinate = CLLocationCoordinate2D(latitude: lat, longitude: lng)
                marke.title = String(format: "#%02d %@", h.number ?? 0, h.name ?? "")
                karte.addAnnotation(marke)
            }
        }

        private func teamsZeichnen(_ karte: MKMapView, _ live: LiveState?) {
            let alt = karte.annotations.filter { $0 is TeamMarke }
            karte.removeAnnotations(alt)

            let alteKreise = karte.overlays.compactMap { $0 as? MKCircle }
            karte.removeOverlays(alteKreise)

            for p in live?.positions ?? [] {
                guard let lat = p.lat, let lng = p.lng else { continue }
                let ort = CLLocationCoordinate2D(latitude: lat, longitude: lng)

                // Unscharfe Positionen als Fläche, nicht als Punkt: Ein Punkt
                // behauptet eine Genauigkeit, die es nicht gibt.
                if let unschaerfe = p.blurM, unschaerfe > 0 {
                    karte.addOverlay(MKCircle(center: ort, radius: unschaerfe),
                                     level: .aboveLabels)
                    continue
                }

                let marke = TeamMarke()
                marke.coordinate = ort
                marke.title = p.display ?? p.callsign
                marke.rolle = p.role ?? ""
                karte.addAnnotation(marke)
            }

            if let finale = live?.finale, finale.active == true,
               let lat = finale.lat, let lng = finale.lng, let r = finale.radiusM, r > 0 {
                let kreis = MKCircle(center: CLLocationCoordinate2D(latitude: lat, longitude: lng),
                                     radius: r)
                kreis.title = "finale"
                karte.addOverlay(kreis, level: .aboveLabels)
            }
        }

        private func rahmen(um feld: FieldMap) -> MKMapRect? {
            var rechteck = MKMapRect.null
            for h in feld.hotspots ?? [] {
                guard let lat = h.lat, let lng = h.lng else { continue }
                let punkt = MKMapPoint(CLLocationCoordinate2D(latitude: lat, longitude: lng))
                rechteck = rechteck.union(MKMapRect(origin: punkt,
                                                    size: MKMapSize(width: 1, height: 1)))
            }
            return rechteck.isNull ? nil : rechteck
        }

        // MARK: - Darstellung

        func mapView(_ karte: MKMapView, rendererFor overlay: MKOverlay) -> MKOverlayRenderer {
            if let kacheln = overlay as? MKTileOverlay {
                return MKTileOverlayRenderer(tileOverlay: kacheln)
            }

            if let flaeche = overlay as? MKPolygon {
                let r = MKPolygonRenderer(polygon: flaeche)
                r.fillColor = UIColor(red: 0.31, green: 0.56, blue: 0.64, alpha: 0.16)
                r.strokeColor = UIColor(red: 0.31, green: 0.56, blue: 0.64, alpha: 0.9)
                r.lineWidth = 1.2
                return r
            }

            if let kreis = overlay as? MKCircle {
                let r = MKCircleRenderer(circle: kreis)
                let finale = kreis.title == "finale"
                let farbe = finale
                    ? UIColor(red: 0.85, green: 0.64, blue: 0.25, alpha: 1)
                    : UIColor(red: 1.00, green: 0.36, blue: 0.28, alpha: 1)
                r.fillColor = farbe.withAlphaComponent(0.12)
                r.strokeColor = farbe.withAlphaComponent(0.8)
                r.lineWidth = finale ? 2 : 1
                return r
            }

            return MKOverlayRenderer(overlay: overlay)
        }

        func mapView(_ karte: MKMapView, viewFor annotation: MKAnnotation) -> MKAnnotationView? {
            if annotation is MKUserLocation { return nil }

            let kennung = annotation is HotspotMarke ? "hotspot" : "team"
            let sicht = karte.dequeueReusableAnnotationView(withIdentifier: kennung)
                ?? MKMarkerAnnotationView(annotation: annotation, reuseIdentifier: kennung)

            sicht.annotation = annotation

            if let marker = sicht as? MKMarkerAnnotationView {
                if annotation is HotspotMarke {
                    marker.markerTintColor = UIColor(red: 0.85, green: 0.64, blue: 0.25, alpha: 1)
                    marker.glyphImage = UIImage(systemName: "mappin")
                    marker.displayPriority = .defaultLow
                } else if let team = annotation as? TeamMarke {
                    marker.markerTintColor = team.rolle == "misterx"
                        ? UIColor(red: 1.00, green: 0.36, blue: 0.28, alpha: 1)
                        : UIColor(red: 0.21, green: 0.75, blue: 0.84, alpha: 1)
                    marker.glyphImage = UIImage(systemName: "person.fill")
                    marker.displayPriority = .required
                }
            }

            return sicht
        }
    }

    final class HotspotMarke: MKPointAnnotation {}

    final class TeamMarke: MKPointAnnotation {
        var rolle = ""
    }
}

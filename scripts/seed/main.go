// Command seed loads a realistic pharmaceutical delivery scenario through the public API:
// depots, fleet, drivers, carriers, two weeks of completed trips, today's live trips with
// telemetry, failed deliveries, tenders and (if the WMS is running) imported WMS shipments.
//
//	go run ./scripts/seed            # API at http://localhost:8090, empty database
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/handarudwiki/pharmacy-tms/pkg/env"
)

type obj = map[string]any

type client struct{ base, token string }

func (c *client) call(method, path string, body any) (obj, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var out obj
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode >= 300 {
		return out, fmt.Errorf("%s %s: %d %v", method, path, res.StatusCode, out["error"])
	}
	return out, nil
}

func (c *client) must(method, path string, body any) obj {
	out, err := c.call(method, path, body)
	if err != nil {
		log.Fatal(err)
	}
	return out
}

func login(base, email, pw string) *client {
	c := &client{base: base}
	c.token = c.must("POST", "/api/auth/login", obj{"email": email, "password": pw})["token"].(string)
	return c
}

func str(o obj, k string) string { s, _ := o[k].(string); return s }
func list(o obj, k string) []obj {
	raw, _ := o[k].([]any)
	out := make([]obj, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.(obj))
	}
	return out
}
func num(o obj, k string) float64 { f, _ := o[k].(float64); return f }

var jkt = time.FixedZone("WIB", 7*3600)

func day(offset int) string { return time.Now().In(jkt).AddDate(0, 0, offset).Format("2006-01-02") }

type place struct {
	name, address string
	lat, lng      float64
}

// Pharmacies, clinics and hospitals across Greater Jakarta.
var places = []place{
	{"Apotek K-24 Kemang", "Jl. Kemang Raya 12, Jakarta Selatan", -6.2607, 106.8136},
	{"RS Siloam Lippo Village", "Jl. Siloam 6, Tangerang", -6.2263, 106.6000},
	{"Guardian Grand Indonesia", "Jl. MH Thamrin 1, Jakarta Pusat", -6.1951, 106.8199},
	{"Apotek Century Senayan", "Jl. Asia Afrika 8, Jakarta Pusat", -6.2272, 106.7990},
	{"RSUD Tangerang", "Jl. Ahmad Yani 9, Tangerang", -6.1745, 106.6337},
	{"Kimia Farma Apotek 201", "Jl. Cikini Raya 60, Jakarta Pusat", -6.1862, 106.8340},
	{"Watsons Pondok Indah", "Jl. Metro Pondok Indah, Jakarta Selatan", -6.2655, 106.7838},
	{"RS Mitra Keluarga Bekasi", "Jl. Ahmad Yani, Bekasi", -6.2471, 107.0003},
	{"Puskesmas Cilandak", "Jl. TB Simatupang, Jakarta Selatan", -6.2915, 106.7997},
	{"Apotek Roxy Cempaka", "Jl. Letjen Suprapto, Jakarta Pusat", -6.1667, 106.8710},
	{"RS Hermina Depok", "Jl. Siliwangi 50, Depok", -6.3910, 106.8250},
	{"Boots Pacific Place", "Jl. Jend. Sudirman 52, Jakarta Selatan", -6.2243, 106.8096},
	{"RSCM Kencana", "Jl. Diponegoro 71, Jakarta Pusat", -6.1966, 106.8473},
	{"RS Pondok Indah Bintaro", "Jl. Bintaro Utama, Tangerang Selatan", -6.2750, 106.7270},
	{"Apotek Melawai Blok M", "Jl. Melawai Raya, Jakarta Selatan", -6.2440, 106.8000},
	{"RS Premier Jatinegara", "Jl. Jatinegara Timur 85, Jakarta Timur", -6.2210, 106.8720},
	{"Klinik Utama Kelapa Gading", "Jl. Boulevard Raya, Jakarta Utara", -6.1575, 106.9055},
	{"RS Omni Pulomas", "Jl. Pulomas Barat VI, Jakarta Timur", -6.1747, 106.8870},
	{"Apotek Generik Cibubur", "Jl. Alternatif Cibubur, Bogor", -6.3720, 106.8930},
	{"RS EMC Cikarang", "Jl. Raya Industri Pasirgombong, Cikarang", -6.2905, 107.1500},
	{"Apotek Kimia Farma Bekasi Barat", "Jl. KH Noer Ali, Bekasi", -6.2390, 106.9900},
	{"RS Hermina Jatinegara", "Jl. Jatinegara Barat 126, Jakarta Timur", -6.2160, 106.8670},
	{"Puskesmas Tanjung Priok", "Jl. Bugis 1, Jakarta Utara", -6.1120, 106.8820},
	{"RSUP Fatmawati", "Jl. RS Fatmawati, Jakarta Selatan", -6.2925, 106.7940},
	{"Apotek Sehat Sentosa", "Jl. Fatmawati 12, Jakarta Selatan", -6.2800, 106.7960},
	{"RS Mayapada Lebak Bulus", "Jl. Lebak Bulus I, Jakarta Selatan", -6.3010, 106.7790},
	{"Klinik Pratama Sehat", "Jl. Salemba Raya 21, Jakarta Pusat", -6.2108, 106.8497},
	{"RS Siloam Kebon Jeruk", "Jl. Raya Perjuangan, Jakarta Barat", -6.1920, 106.7650},
	{"Apotek Roxy Cengkareng", "Jl. Daan Mogot, Jakarta Barat", -6.1530, 106.7350},
	{"RSUD Cengkareng", "Jl. Kamal Raya, Jakarta Barat", -6.1320, 106.7310},
	{"Apotek Century Kalimalang", "Jl. Kalimalang, Jakarta Timur", -6.2470, 106.9220},
	{"RS Awal Bros Bekasi", "Jl. KH Noer Ali, Bekasi", -6.2440, 106.9930},
	{"Puskesmas Cikarang Utara", "Jl. Raya Cikarang Cibarusah", -6.2620, 107.1460},
	{"Apotek K-24 Tambun", "Jl. Sultan Hasanudin, Tambun", -6.2620, 107.0560},
	{"RS Hermina Bekasi", "Jl. Kemakmuran 39, Bekasi", -6.2370, 106.9960},
	{"Watsons Kota Kasablanka", "Jl. Casablanca Raya, Jakarta Selatan", -6.2240, 106.8430},
	{"Guardian Mall Kelapa Gading", "Jl. Boulevard Kelapa Gading, Jakarta Utara", -6.1580, 106.9080},
	{"RS Mitra Keluarga Cibubur", "Jl. Alternatif Cibubur, Depok", -6.3690, 106.8970},
	{"Apotek Bunda", "Jl. Pramuka 9, Jakarta Timur", -6.2000, 106.8680},
	{"Klinik Medika Plaza", "Jl. HR Rasuna Said, Jakarta Selatan", -6.2260, 106.8300},
}

type kind struct {
	temp       string
	controlled bool
	weight     func() float64
	note       string
}

func main() {
	base := flag.String("api", "http://localhost:8090", "gateway base URL")
	flag.Parse()
	rnd := rand.New(rand.NewPCG(11, 29))

	admin := login(*base, env.Get("TMS_ADMIN_EMAIL", "admin@tms.local"), env.Get("TMS_ADMIN_PASSWORD", "admin123"))
	if len(list(admin.must("GET", "/api/depots", nil), "depots")) > 0 {
		log.Fatal("database already has depots; seed expects an empty database (run scripts/reset-db.sh)")
	}

	log.Println("creating users")
	staff := []struct {
		email, name string
		roles       []string
	}{
		{"transport@tms.local", "Maya Kusuma", []string{"TRANSPORT_MANAGER"}},
		{"dispatcher@tms.local", "Hendra Gunawan", []string{"DISPATCHER"}},
		{"fleet@tms.local", "Agus Salim", []string{"FLEET_MANAGER"}},
		{"carrier@tms.local", "Lina Marlina", []string{"CARRIER_MANAGER"}},
		{"cs@tms.local", "Putri Ayu", []string{"CUSTOMER_SERVICE"}},
		{"analyst@tms.local", "Rizky Pratama", []string{"TRANSPORT_ANALYST"}},
		{"integration@tms.local", "WMS Connector", []string{"INTEGRATION_ADMIN"}},
	}
	type drv struct {
		email, name, license string
		expiry               int
		cold, controlled     bool
		status               string
	}
	drivers := []drv{
		{"budi@tms.local", "Budi Hartono", "SIM-B1-310245", 700, true, true, "ACTIVE"},
		{"slamet@tms.local", "Slamet Riyadi", "SIM-B1-221876", 520, true, false, "ACTIVE"},
		{"andi@tms.local", "Andi Wijaya", "SIM-B1-198734", 410, false, false, "ACTIVE"},
		{"rudi@tms.local", "Rudi Hermawan", "SIM-B1-176543", 900, false, true, "ACTIVE"},
		{"yusuf@tms.local", "Yusuf Maulana", "SIM-B1-165432", 18, false, false, "ACTIVE"},
		{"joko@tms.local", "Joko Santoso", "SIM-B1-154321", 600, true, false, "ACTIVE"},
		{"dedi@tms.local", "Dedi Kurniawan", "SIM-B1-143210", -12, false, false, "ACTIVE"},
		{"eko@tms.local", "Eko Prasetyo", "SIM-B1-132109", 300, false, false, "ON_LEAVE"},
	}
	for _, u := range staff {
		admin.must("POST", "/api/users", obj{"email": u.email, "name": u.name, "password": "password123", "roles": u.roles})
	}
	for _, d := range drivers {
		admin.must("POST", "/api/users", obj{"email": d.email, "name": d.name, "password": "password123", "roles": []string{"DRIVER"}})
	}
	mgr := login(*base, "transport@tms.local", "password123")
	dsp := login(*base, "dispatcher@tms.local", "password123")
	flt := login(*base, "fleet@tms.local", "password123")
	car := login(*base, "carrier@tms.local", "password123")

	log.Println("creating depots, fleet and drivers")
	ckr := admin.must("POST", "/api/depots", obj{"code": "CKR", "name": "Cikarang Distribution Center", "address": "Jl. Industri Raya No. 8, Cikarang, Bekasi", "lat": -6.2850, "lng": 107.1700})
	depot := str(ckr, "id")
	jktDepot := str(admin.must("POST", "/api/depots", obj{"code": "SNT", "name": "Sunter Cross-dock", "address": "Jl. Danau Sunter Utara, Jakarta Utara", "lat": -6.1380, "lng": 106.8650}), "id")
	type veh struct {
		plate, kind    string
		kg, vol        float64
		controlled     bool
		kmpl           float64
		status, reason string
		depot          string
	}
	vehicles := []veh{
		{"B 9101 KJT", "BOX_TRUCK", 2200, 14000, false, 6, "AVAILABLE", "", depot},
		{"B 9102 KJT", "VAN", 850, 4500, true, 9, "AVAILABLE", "", depot},
		{"B 9103 KJT", "VAN", 850, 4500, false, 9, "AVAILABLE", "", depot},
		{"B 9104 KJT", "VAN", 850, 4500, false, 9, "MAINTENANCE", "Brake pad replacement", depot},
		{"B 9201 KJT", "REEFER_VAN", 650, 3200, true, 8, "AVAILABLE", "", depot},
		{"B 9202 KJT", "REEFER_TRUCK", 1500, 8000, false, 6.5, "AVAILABLE", "", depot},
		{"B 4401 KJT", "MOTORBIKE", 60, 140, false, 35, "AVAILABLE", "", depot},
		{"B 9301 SNT", "VAN", 850, 4500, false, 9, "AVAILABLE", "", jktDepot},
	}
	vehicleIDs := map[string]string{}
	for i, v := range vehicles {
		res := flt.must("POST", "/api/vehicles", obj{"depot_id": v.depot, "plate": v.plate, "vehicle_type": v.kind, "capacity_kg": v.kg,
			"capacity_volume_l": v.vol, "controlled_certified": v.controlled, "fuel_km_per_liter": v.kmpl, "odometer_km": 18000 + i*7300,
			"next_service_date": day(10 + i*9)})
		vehicleIDs[v.plate] = str(res, "id")
		if v.status != "AVAILABLE" {
			flt.must("POST", "/api/vehicles/"+str(res, "id")+"/status", obj{"status": v.status, "reason": v.reason})
		}
	}
	for _, m := range []struct {
		plate, kind, desc string
		cost              float64
		ago               int
	}{
		{"B 9104 KJT", "REPAIR", "Front brake pads and discs replaced", 2850000, 0},
		{"B 9201 KJT", "INSPECTION", "Reefer unit calibration (2–8 °C), certificate renewed", 1200000, 6},
		{"B 9101 KJT", "SERVICE", "20,000 km service, oil and filters", 1650000, 11},
		{"B 9202 KJT", "TIRES", "Rear tyres replaced", 4200000, 19},
	} {
		flt.must("POST", "/api/maintenance", obj{"vehicle_id": vehicleIDs[m.plate], "maintenance_type": m.kind, "description": m.desc,
			"cost": m.cost, "performed_on": day(-m.ago), "next_due_date": day(80 - m.ago)})
	}
	for _, d := range drivers {
		res := flt.must("POST", "/api/drivers", obj{"depot_id": depot, "name": d.name, "phone": fmt.Sprintf("+62 812-%04d-%04d", rnd.IntN(9999), rnd.IntN(9999)),
			"license_number": d.license, "license_class": "B1", "license_expiry": day(d.expiry), "cold_chain_certified": d.cold,
			"controlled_certified": d.controlled, "user_email": d.email})
		if d.status != "ACTIVE" {
			flt.must("POST", "/api/drivers/"+str(res, "id")+"/status", obj{"status": d.status})
		}
	}

	log.Println("creating carriers")
	for _, c := range []obj{
		{"code": "JNE", "name": "JNE Logistics", "service_level": "NEXT_DAY", "base_rate": 25000, "rate_per_kg": 3200, "rate_per_km": 450, "on_time_pct": 93, "transit_days": 1, "contact": "corporate@jne.co.id"},
		{"code": "SCP", "name": "SiCepat Ekspres", "service_level": "NEXT_DAY", "base_rate": 20000, "rate_per_kg": 3500, "rate_per_km": 400, "on_time_pct": 91, "transit_days": 1, "contact": "b2b@sicepat.com"},
		{"code": "LLM", "name": "Lalamove Business", "service_level": "SAME_DAY", "base_rate": 45000, "rate_per_kg": 1500, "rate_per_km": 3500, "on_time_pct": 96, "transit_days": 0, "contact": "business@lalamove.com"},
		{"code": "MBS", "name": "MBS Cold Chain Logistics", "service_level": "SAME_DAY", "supports_cold": true, "supports_controlled": true, "base_rate": 90000, "rate_per_kg": 6500, "rate_per_km": 2800, "cold_surcharge_pct": 25, "on_time_pct": 98, "transit_days": 0, "contact": "ops@mbscoldchain.co.id"},
		{"code": "DLV", "name": "Deliveree", "service_level": "SAME_DAY", "base_rate": 60000, "rate_per_kg": 1200, "rate_per_km": 3000, "on_time_pct": 94, "transit_days": 0, "contact": "corporate@deliveree.com"},
	} {
		car.must("POST", "/api/carriers", c)
	}

	kinds := []kind{
		{"AMBIENT", false, func() float64 { return 15 + rnd.Float64()*70 }, "OTC & general pharma"},
		{"AMBIENT", false, func() float64 { return 20 + rnd.Float64()*120 }, "Medical devices & consumables"},
		{"AMBIENT", false, func() float64 { return 10 + rnd.Float64()*40 }, "Cosmetics replenishment"},
		{"COLD", false, func() float64 { return 8 + rnd.Float64()*25 }, "Vaccines & insulin — keep 2–8 °C"},
		{"AMBIENT", true, func() float64 { return 3 + rnd.Float64()*6 }, "Controlled drugs — double signature"},
	}
	pick := func() kind {
		switch r := rnd.IntN(20); {
		case r < 3:
			return kinds[3]
		case r == 3:
			return kinds[4]
		case r < 9:
			return kinds[1]
		case r < 12:
			return kinds[2]
		}
		return kinds[0]
	}
	windows := [][2]string{{"08:00", "17:00"}, {"09:00", "15:00"}, {"08:00", "12:00"}, {"13:00", "17:00"}, {"10:00", "16:00"}}
	newShipment := func(required string) obj {
		p := places[rnd.IntN(len(places))]
		k := pick()
		w := windows[rnd.IntN(len(windows))]
		weight := float64(int(k.weight()*10)) / 10
		return dsp.must("POST", "/api/shipments", obj{"depot_id": depot, "consignee_name": p.name, "address": p.address, "city": "Jabodetabek",
			"lat": p.lat + (rnd.Float64()-0.5)*0.004, "lng": p.lng + (rnd.Float64()-0.5)*0.004, "consignee_phone": fmt.Sprintf("+62 21-%07d", rnd.IntN(9999999)),
			"window_start": w[0], "window_end": w[1], "required_date": required, "weight_kg": weight, "packages": 1 + int(weight/12),
			"temperature_class": k.temp, "controlled": k.controlled, "priority": []int{1, 2, 3, 3, 3, 4}[rnd.IntN(6)], "notes": k.note})
	}
	driverClient := map[string]*client{}
	asDriver := func(email string) *client {
		if driverClient[email] == nil {
			driverClient[email] = login(*base, email, "password123")
		}
		return driverClient[email]
	}
	receivers := []string{"Sri Wahyuni (apoteker)", "dr. Andreas", "Rina (gudang farmasi)", "Bpk. Haryanto", "Ibu Lestari", "Dewi (admin)", "Teguh (security)"}

	// runTrip executes a released trip as its driver. failAt marks one stop as a failed delivery.
	runTrip := func(trip obj, failAt int, telemetry bool) {
		id := str(trip, "id")
		dc := asDriver(str(trip, "driver_email"))
		dc.must("POST", "/api/trips/"+id+"/start", nil)
		lat, lng := num(trip, "depot_lat"), num(trip, "depot_lng")
		for i, st := range list(trip, "stops") {
			if telemetry {
				for k := 1; k <= 3; k++ {
					f := float64(k) / 3
					tc := 3.5 + rnd.Float64()*2.5
					dc.must("POST", "/api/trips/"+id+"/telemetry", obj{"lat": lat + (num(st, "lat")-lat)*f, "lng": lng + (num(st, "lng")-lng)*f,
						"temperature_c": tc, "has_temperature": str(trip, "temperature_class") == "COLD", "speed_kmh": 18 + rnd.Float64()*30})
				}
			}
			lat, lng = num(st, "lat"), num(st, "lng")
			path := "/api/trips/" + id + "/stops/" + str(st, "id")
			dc.must("POST", path+"/arrive", nil)
			if i == failAt {
				dc.must("POST", path+"/fail", obj{"reason": []string{"CUSTOMER_ABSENT", "CLOSED", "ADDRESS_NOT_FOUND"}[rnd.IntN(3)],
					"note": "Receiving desk closed, called consignee without answer", "reschedule": true})
				continue
			}
			dc.must("POST", path+"/deliver", obj{"receiver_name": receivers[rnd.IntN(len(receivers))], "signature_captured": true,
				"note": "Received in good condition", "lat": lat, "lng": lng})
		}
		dc.must("POST", "/api/trips/"+id+"/complete", nil)
	}

	log.Println("simulating the past two weeks")
	var past []pastTrip
	for offset := -13; offset <= -1; offset++ {
		if time.Now().In(jkt).AddDate(0, 0, offset).Weekday() == time.Sunday {
			continue
		}
		for i := 0; i < 7+rnd.IntN(5); i++ {
			newShipment(day(offset))
		}
		plan := dsp.must("POST", "/api/planning/auto", obj{"depot_id": depot, "plan_date": day(offset)})
		for j, t := range list(plan, "trips") {
			trip := dsp.must("POST", "/api/trips/"+str(t, "id")+"/release", nil)
			fail := -1
			if (offset+j)%4 == 0 {
				fail = rnd.IntN(len(list(trip, "stops")))
			}
			runTrip(trip, fail, j == 0)
			past = append(past, pastTrip{str(t, "id"), offset})
		}
	}

	log.Println("planning and running today's trips")
	for i := 0; i < 22; i++ {
		newShipment(day(0))
	}
	plan := dsp.must("POST", "/api/planning/auto", obj{"depot_id": depot, "plan_date": day(0)})
	trips := list(plan, "trips")
	for i, t := range trips {
		if i >= 3 {
			break // the rest stay planned for the dispatcher to release
		}
		trip := dsp.must("POST", "/api/trips/"+str(t, "id")+"/release", nil)
		if i == 2 {
			continue // released, driver has not left yet
		}
		id := str(trip, "id")
		dc := asDriver(str(trip, "driver_email"))
		dc.must("POST", "/api/trips/"+id+"/start", nil)
		stops := list(trip, "stops")
		lat, lng := num(trip, "depot_lat"), num(trip, "depot_lng")
		cold := str(trip, "temperature_class") == "COLD"
		done := len(stops) / 2
		for k, st := range stops[:done] {
			for step := 1; step <= 4; step++ {
				f := float64(step) / 4
				tc := 3.8 + rnd.Float64()*2
				if cold && k == 1 && step == 2 {
					tc = 9.4 // reefer door left open while unloading — excursion
				}
				dc.must("POST", "/api/trips/"+id+"/telemetry", obj{"lat": lat + (num(st, "lat")-lat)*f, "lng": lng + (num(st, "lng")-lng)*f,
					"temperature_c": tc, "has_temperature": cold, "speed_kmh": 15 + rnd.Float64()*30})
			}
			lat, lng = num(st, "lat"), num(st, "lng")
			path := "/api/trips/" + id + "/stops/" + str(st, "id")
			dc.must("POST", path+"/arrive", nil)
			if i == 1 && k == done-1 {
				dc.must("POST", path+"/fail", obj{"reason": "CUSTOMER_ABSENT", "note": "Pharmacy closed for Friday prayers, will retry", "reschedule": true})
				continue
			}
			dc.must("POST", path+"/deliver", obj{"receiver_name": receivers[rnd.IntN(len(receivers))], "signature_captured": true, "note": "OK", "lat": lat, "lng": lng})
		}
		if done < len(stops) {
			next := stops[done]
			dc.must("POST", "/api/trips/"+id+"/telemetry", obj{"lat": lat + (num(next, "lat")-lat)*0.4, "lng": lng + (num(next, "lng")-lng)*0.4,
				"temperature_c": 4.6, "has_temperature": cold, "speed_kmh": 24})
		}
	}

	log.Println("tendering overflow to carriers")
	var unplanned []string
	for _, u := range list(plan, "unplanned") {
		unplanned = append(unplanned, str(u, "shipment_id"))
	}
	for i := 0; i < 5; i++ {
		unplanned = append(unplanned, str(newShipment(day(1)), "id"))
	}
	if len(unplanned) >= 4 {
		t := car.must("POST", "/api/tenders", obj{"depot_id": depot, "shipment_ids": unplanned[:2]})
		for _, b := range list(t, "bids") {
			if b["compliant"] == true {
				car.must("POST", "/api/tenders/"+str(t, "id")+"/award", obj{"bid_id": str(b, "id")})
				break
			}
		}
		car.must("POST", "/api/tenders", obj{"depot_id": depot, "shipment_ids": unplanned[2:4]})
	}
	for i := 0; i < 6; i++ {
		newShipment(day(1))
	}

	if res, err := (&client{base: *base, token: login(*base, "integration@tms.local", "password123").token}).call("POST", "/api/integrations/wms/sync", obj{"depot_id": depot}); err == nil {
		log.Printf("imported %v shipments from the WMS", res["imported"])
	} else {
		log.Printf("WMS sync skipped (%v)", err)
	}

	log.Println("back-dating history")
	backdate(past)
	_ = mgr
	log.Printf("done. Log in as transport@tms.local / password123 (or admin@tms.local / admin123); drivers e.g. budi@tms.local / password123")
}

// backdate moves the executed past trips (and their shipments, events, exceptions and telemetry)
// onto their plan dates so dashboards show a realistic history. Demo data only.
type pastTrip struct {
	id     string
	offset int
}

func backdate(past []pastTrip) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, env.DatabaseURL())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback(ctx)
	exec := func(sql string, args ...any) {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			log.Fatalf("%v: %s", err, sql)
		}
	}
	for i, p := range past {
		exec(`UPDATE planning.trips SET created_at = planned_start - interval '14 hours', started_at = planned_start + interval '10 minutes',
			completed_at = planned_start + make_interval(mins => duration_min + 25) WHERE id=$1`, p.id)
		exec(`UPDATE planning.stops SET arrived_at = eta - interval '4 minutes' + make_interval(mins => (sequence * 3) % 11),
			completed_at = eta + interval '9 minutes' + make_interval(mins => (sequence * 3) % 11) WHERE trip_id=$1`, p.id)
		exec(`UPDATE planning.telemetry t SET recorded_at = tr.planned_start + make_interval(mins => (15 + x.rn * 12)::int)
			FROM (SELECT id, row_number() OVER (ORDER BY recorded_at) AS rn FROM planning.telemetry WHERE trip_id=$1) x,
			planning.trips tr WHERE t.id = x.id AND tr.id = $1`, p.id)
		// Shipments on the trip: creation the afternoon before, delivery at the stop's completion time.
		lateShift := ""
		if i%9 == 4 {
			lateShift = " + interval '1 day'"
		}
		exec(`UPDATE shipment.shipments s SET created_at = t.planned_start - interval '18 hours', delivered_at = st.completed_at`+lateShift+`,
			updated_at = COALESCE(st.completed_at, t.planned_start)`+lateShift+`
			FROM planning.stops st JOIN planning.trips t ON t.id = st.trip_id
			WHERE st.trip_id=$1 AND s.id = st.shipment_id`, p.id)
		exec(`UPDATE shipment.shipment_events e SET created_at = e.created_at - make_interval(days => $2)
			WHERE e.shipment_id IN (SELECT shipment_id FROM planning.stops WHERE trip_id=$1)
			AND e.created_at > now() - interval '1 hour'`, p.id, -p.offset)
		exec(`UPDATE shipment.exceptions SET created_at = created_at - make_interval(days => $2) WHERE trip_id=$1`, p.id, -p.offset)
	}
	// Deliveries were recorded "now" and back-dated above; drop late flags that no longer apply.
	exec(`DELETE FROM shipment.exceptions e USING shipment.shipments s WHERE e.exception_type='LATE_DELIVERY'
		AND s.id::text = e.shipment_id AND s.delivered_at::date <= s.required_date`)
	if err := tx.Commit(ctx); err != nil {
		log.Fatal(err)
	}
}

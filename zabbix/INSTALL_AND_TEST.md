# คู่มือติดตั้งและทดสอบ Template (Step by Step)

คู่มือนี้พาตั้งแต่ยกเซิร์ฟเวอร์ Zabbix 7.0 ขึ้นมาทดสอบ → import template →
ตั้งค่าเชื่อม FusionCompute → ทดสอบทีละชั้น → re-export ก่อนส่ง PR

> มี 2 แนวทาง: **(ก)** ทดสอบ logic แบบ offline (ไม่ต้องมี Zabbix/VRM จริง) และ
> **(ข)** ทดสอบจริงบน Zabbix 7.0 ทำ (ก) ก่อนเสมอเพราะเร็วและไม่ต้องเตรียมอะไร

---

## ส่วน ก. ทดสอบ logic แบบ offline (5 นาที)

ต้องมีแค่ `python3` และ `node`

```bash
cd zabbix/test
python3 extract_scripts.py plain     # ดึง JS จริงจาก template + ใส่ค่าทดสอบ
node harness.js                      # ควรได้ "32 passed, 0 failed"
python3 extract_scripts.py sha256    # ทดสอบ auth อีกโหมด
node harness.js
```

ผ่านแล้วแปลว่า logic การ login / pagination / parse / preprocessing ถูกต้อง
เหลือแค่ยืนยันฝั่ง Zabbix จริง (ส่วน ข)

---

## ส่วน ข. ทดสอบจริงบน Zabbix 7.0

### ขั้นที่ 1 — เตรียม Zabbix 7.0 ทดสอบด้วย Docker (เร็วสุด)

ต้องมี Docker + Docker Compose สร้างไฟล์ `docker-compose.yml`:

```yaml
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: zabbix
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: zabbix_pwd
  server:
    image: zabbix/zabbix-server-pgsql:alpine-7.0-latest
    environment:
      DB_SERVER_HOST: db
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: zabbix_pwd
      POSTGRES_DB: zabbix
    depends_on: [db]
    ports: ["10051:10051"]
  web:
    image: zabbix/zabbix-web-nginx-pgsql:alpine-7.0-latest
    environment:
      DB_SERVER_HOST: db
      POSTGRES_USER: zabbix
      POSTGRES_PASSWORD: zabbix_pwd
      POSTGRES_DB: zabbix
      ZBX_SERVER_HOST: server
      PHP_TZ: Asia/Bangkok
    depends_on: [server]
    ports: ["8080:8080"]
```

```bash
docker compose up -d
# รอ ~1 นาที แล้วเปิด http://localhost:8080
# login เริ่มต้น:  Admin / zabbix
```

> **สำคัญ:** Script item รันบน **Zabbix server** ดังนั้น container `server`
> ต้อง route ไปถึง FusionCompute VRM (พอร์ต 7443) ได้ ถ้า VRM อยู่ในวงแลน
> ปกติ Docker bridge ออกได้อยู่แล้ว ถ้าต่อไม่ถึงให้เพิ่ม
> `network_mode: host` ที่ service `server` หรือรัน server บนเครื่องในวง

### ขั้นที่ 2 — สร้าง user อ่านอย่างเดียวบน FusionCompute

1. เข้า FusionCompute (VRM) ด้วยสิทธิ์ admin
2. สร้าง user ใหม่ เช่น `zbx_monitor` ให้สิทธิ์แบบ **read-only / ผู้ดูแลระบบระดับอ่าน**
3. จดรหัสผ่านไว้ใช้กับ macro `{$FC.PASSWORD}`

### ขั้นที่ 3 — จัดการ TLS (self-signed cert ของ VRM)

`HttpRequest` ของ Zabbix verify certificate กับ CA store ของเครื่อง server
ถ้า VRM ใช้ self-signed (ปกติเป็น) ให้เพิ่ม cert เข้า trust store ของ
container/เครื่องที่รัน zabbix-server:

```bash
# ดึง cert ของ VRM
openssl s_client -connect <VRM-IP>:7443 -showcerts </dev/null 2>/dev/null \
  | openssl x509 -outform PEM > fusioncompute.crt

# คัดลอกเข้า container server แล้ว update CA (ภาพ alpine)
docker cp fusioncompute.crt <server-container>:/usr/local/share/ca-certificates/
docker exec <server-container> update-ca-certificates
docker compose restart server
```

> ถ้าทดสอบรอบแรกแล้วเจอ error ประเภท SSL/TLS ที่ Script item ให้กลับมาทำขั้นนี้

### ขั้นที่ 4 — Import template

1. เข้าเว็บ → **Data collection → Templates**
2. กดปุ่ม **Import** (มุมขวาบน)
3. เลือกไฟล์
   `zabbix/Virtualization/Huawei_FusionCompute/7.0/template_huawei_fusioncompute_http.yaml`
4. ติ๊ก *Create new* / *Update existing* แล้วกด **Import**
5. ✅ **เช็คพอยต์ 1:** ต้องขึ้น "Imported successfully" ถ้า error แสดงว่า
   schema มีปัญหา — คัดลอกข้อความ error มาให้ผมแก้

### ขั้นที่ 5 — สร้าง Host แล้ว link template

1. **Data collection → Hosts → Create host**
2. **Host name:** เช่น `FusionCompute-Lab`
3. **Templates:** เพิ่ม *Huawei FusionCompute by HTTP*
4. **Host groups:** เลือก/สร้างกลุ่ม เช่น `Virtualization`
5. **Interfaces:** ไม่จำเป็น (เราใช้ `{$FC.HOST}`) แต่ถ้าใส่ ให้ใส่ IP ของ VRM
6. แท็บ **Macros → Inherited and host macros** กรอก:
   - `{$FC.HOST}` = IP/FQDN ของ VRM
   - `{$FC.USER}` = `zbx_monitor`
   - `{$FC.PASSWORD}` = รหัสผ่าน
   - `{$FC.AUTH.MODE}` = `plain` (ถ้าล็อกอินไม่ผ่านค่อยลอง `sha256`)
   - (ถ้าจำเป็น) `{$FC.API.VERSION}` = เวอร์ชันที่ตรงกับ VRM ของคุณ
7. กด **Add**

### ขั้นที่ 6 — ทดสอบ Script item ทีละตัว (จุดสำคัญที่สุด)

1. **Data collection → Hosts →** คลิก **Items** ของโฮสต์
2. เปิด item **"FusionCompute: Get hosts"**
3. กดปุ่ม **Test** (ขวาบน) → **Get value and test**
4. ✅ **เช็คพอยต์ 2:** ควรได้ JSON array ของ host กลับมา
   - ถ้าได้ → auth + เครือข่าย + TLS ใช้ได้ทั้งหมด
   - ถ้า error → ดูตารางแก้ปัญหาท้ายเอกสาร
5. ทดสอบซ้ำกับ *Get VMs / Get datastores / Get clusters / Get summary*

> ทำขั้นนี้ให้ผ่านก่อนค่อยไปต่อ เพราะ item อื่นทั้งหมดต่อยอดจาก 5 ตัวนี้

### ขั้นที่ 7 — ตรวจ Discovery (LLD)

1. **Data collection → Hosts → Discovery** ของโฮสต์
2. จะเห็น 4 rule: Host / VM / Datastore / Cluster discovery
3. คลิก **Execute now** ที่แต่ละ rule (หรือรอ ~5 นาที)
4. กลับไปดู **Items** → ควรเห็น item ที่ discover มา เช่น
   `Host [CNA01]: Status`, `Datastore [IPSAN]: Space used, %`
5. ✅ **เช็คพอยต์ 3:** item เหล่านี้มีค่า (ไม่ใช่ *Not supported*)

> ค่าจะปรากฏหลัง master item เก็บข้อมูลรอบถัดไป ถ้ายังว่างให้กด
> *Execute now* ที่ master item ก่อน

### ขั้นที่ 8 — ตรวจ Latest data และ Triggers

1. **Monitoring → Latest data** → กรองด้วยโฮสต์ → ดูค่าที่ไหลเข้า
2. **Data collection → Hosts → Triggers** → ตรวจว่า trigger ถูกสร้าง
3. ทดลองให้ trigger ทำงาน เช่น ปิด/ปรับ macro
   `{$FC.DATASTORE.PUSED.MAX.WARN}` ให้ต่ำกว่าค่าจริงชั่วคราว แล้วดูว่ามี
   problem เด้งใน **Monitoring → Problems**
4. ✅ **เช็คพอยต์ 4:** trigger เด้ง/หายตามค่าจริง

### ขั้นที่ 9 — Re-export ก่อนส่ง PR

หลังทุกอย่างทำงาน ให้ export กลับจาก Zabbix เพื่อให้ได้ YAML รูปแบบ canonical
(เรียงลำดับ/ฟิลด์ตามมาตรฐานที่ server สร้าง — สิ่งที่ upstream คาดหวัง):

1. **Data collection → Templates →** ติ๊กเลือก *Huawei FusionCompute by HTTP*
2. ปุ่มล่าง **Export → YAML**
3. นำไฟล์นี้ไปวางทับ
   `Virtualization/Huawei_FusionCompute/7.0/template_huawei_fusioncompute_http.yaml`
4. ทำตาม `CONTRIBUTING_TO_ZABBIX.md` เพื่อ fork + เปิด PR

---

## ตารางแก้ปัญหา (Troubleshooting)

| อาการ | สาเหตุที่เป็นไปได้ | วิธีแก้ |
|-------|------------------|--------|
| Import ไม่ผ่าน / schema error | YAML field ไม่ตรงเวอร์ชัน | คัดลอกข้อความ error มา; ตรวจว่าเป็น Zabbix 7.0 |
| Script item: `login failed` HTTP 401 | user/pass ผิด หรือ auth mode ไม่ตรง | สลับ `{$FC.AUTH.MODE}` plain ↔ sha256; เช็ค user |
| Script item: error มีเลข `10000022` | API version ไม่ตรง VRM | ตั้ง `{$FC.API.VERSION}` ให้ตรง (v8.0/v6.5/v6.3/...) |
| Script item: SSL/TLS error | self-signed cert ไม่ถูก trust | ทำขั้นที่ 3 (เพิ่ม cert เข้า CA store ของ server) |
| Script item: timeout / connection refused | server เข้าไม่ถึง VRM:7443 | เช็ค routing/firewall; ลอง `network_mode: host` |
| `no sites returned` | user ไม่มีสิทธิ์เห็น site | ให้สิทธิ์ read กับ user บน VRM |
| LLD ไม่เจออะไร | master item ยังไม่มีข้อมูล | กด *Execute now* ที่ master item ก่อน |
| VM vCPU/Memory = 0 | API list ไม่ส่ง `vmConfig` | ปกติตามเวอร์ชัน VRM (ดูหมายเหตุใน README) |
| item เป็น *Not supported* | JSONPath ไม่ match หรือ master error | เปิด master item ดู value ว่ามี field นั้นจริงไหม |

---

## สรุป checklist ก่อนเปิด PR

- [ ] offline harness ผ่าน (32 passed)
- [ ] import เข้า Zabbix 7.0 ได้ไม่มี error (เช็คพอยต์ 1)
- [ ] Script item ทั้ง 5 ตัวคืน JSON จริง (เช็คพอยต์ 2)
- [ ] LLD discover เจอ host/vm/datastore/cluster (เช็คพอยต์ 3)
- [ ] trigger ทำงานตามค่าจริง (เช็คพอยต์ 4)
- [ ] re-export YAML จาก Zabbix แล้ว

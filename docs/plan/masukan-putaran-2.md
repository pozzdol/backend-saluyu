# Masukan Putaran 2 dan Rencana Fase 12–17

Ditulis setelah Fase 11a (commit backend `d9367e2`, frontend `cbaca0c`). Aplikasi dicoba ulang dari dua sudut pandang:

- **Persona C, praktisi finance.** Bekerja di bidang keuangan (FP&A/akuntansi), mengatur keuangan pribadi dan keluarga dengan disiplin, terbiasa dengan Excel, laporan bulanan, dan pajak.
- **Persona D, pengguna awam.** Tidak paham finance sama sekali. Ingin tahu uangnya ke mana dan tidak mau repot.

ID baru memakai C dan D, supaya tidak bentrok dengan A/B di [masukan-dan-fase-lanjutan.md](masukan-dan-fase-lanjutan.md). Kolom "Sekarang" menyebut kondisi kode saat ini.

---

## 1. Persona C: praktisi finance

> "Saya terkesan: double-entry yang benar, tutup buku dengan jurnal pembalik, arus kas yang cocok dengan neraca, impor mutasi tanpa dobel, cicilan anuitas yang presisi. Ini sudah lebih rapi dari kebanyakan aplikasi. Yang kurang sekarang bukan fondasi, tapi hal-hal yang membuat saya bisa berhenti memakai Excel sama sekali."

### Yang sudah baik (pertahankan)
- Uang disimpan dalam sen, semua entry seimbang, jejak revisi ada.
- Tutup buku + jurnal pembalik + jurnal manual: koreksi bisa diaudit.
- Tagihan kartu (tanggal cetak, minimum, status), cicilan flat/anuitas dengan pokok dan bunga terpisah.
- Arus kas metode langsung, perbandingan dengan periode lalu dan rata-rata 3 periode, ekspor Excel.
- Impor mutasi dengan hash per baris dan pencocokan dengan catatan manual.

### Pencatatan
| ID | Masukan | Sekarang |
|---|---|---|
| C1 | **Satu transaksi, banyak kategori (split).** Struk supermarket berisi belanja dapur, perlengkapan rumah, dan jajan. Harus bisa dipecah tanpa jurnal manual. | Hanya lewat Jurnal Manual. |
| C2 | **Slip gaji lengkap.** Gaji bruto, PPh 21, BPJS Kesehatan, BPJS JHT/JP, potongan koperasi, lalu netto yang masuk rekening. Laporan pajak dan "penghasilan setahun" harus pakai bruto, bukan netto. | Gaji dicatat netto; potongan tidak terlihat. |
| C3 | **Reimburse kantor.** Uang pribadi yang dipakai untuk kantor adalah piutang ke kantor, bukan pengeluaran, dan harus mudah ditandai lalu dilunasi saat reimburse cair. | Bisa lewat Kontak (kantor sebagai kontak), tapi tidak terasa sebagai fitur. |
| C4 | **Tag lintas kategori** (misalnya "Liburan Bali 2026", "Renovasi") untuk melihat total satu proyek dari banyak kategori. | Tidak ada (rencana 11b). |
| C5 | **Lampiran struk dan bukti transfer** untuk audit dan klaim reimburse. | Tidak ada (rencana 11b, menunggu keputusan penyimpanan). |
| C6 | **Multi-mata uang.** Gaji atau belanja dalam USD/SGD, kartu kredit yang ditagih dalam rupiah dengan kurs berbeda. | Hanya rupiah. |

### Investasi dan kekayaan
| ID | Masukan | Sekarang |
|---|---|---|
| C7 | **Portofolio sederhana:** reksa dana (unit × NAB), saham (lot × harga), emas (gram × harga), deposito. Nilai pasar diperbarui manual, untung/rugi belum terealisasi dipisah dari pendapatan rutin. | Investasi hanya saldo akun; tidak ada unit dan harga. |
| C8 | **Aset non-keuangan:** rumah, kendaraan, dengan penyusutan atau nilai taksiran, supaya kekayaan bersih lengkap. | Bisa dicatat sebagai akun Asset, tanpa penyusutan atau revaluasi. |
| C9 | **Aset yang tidak likuid** (JHT BPJS, DPLK, deposito terkunci) harus dibedakan dari kas, supaya dana darurat tidak terlihat lebih besar dari kenyataan. | Hanya Investasi dan Piutang yang dikeluarkan dari kas. |

### Laporan dan perencanaan
| ID | Masukan | Sekarang |
|---|---|---|
| C10 | **Bulan kalender dan tahun penuh**, di samping periode gajian. Laporan tahunan per bulan (12 kolom) adalah format yang saya pakai di Excel. | Hanya periode gajian. |
| C11 | **Tanggal gajian 29–31 atau "hari kerja terakhir".** Banyak kantor membayar tanggal 30 atau hari terakhir. | Hanya tanggal 1–28. |
| C12 | **Proyeksi arus kas 30/60/90 hari:** saldo hari ini + transaksi berulang + angsuran + tagihan kartu + sisa anggaran. "Apakah saya kekurangan uang sebelum gajian?" | Tidak ada. |
| C13 | **Kelompok kategori:** kebutuhan pokok / gaya hidup / tabungan-investasi, untuk rasio 50/30/20 dan rasio pengeluaran wajib (A16 yang tertunda). | Tidak ada. |
| C14 | **Anggaran tahunan dan biaya tahunan diratakan** (asuransi, pajak kendaraan, SPP): disisihkan per bulan, dipakai sekali setahun. Sisa anggaran bisa dibawa (amplop). | Anggaran per periode tanpa sisa terbawa; tujuan bisa dipakai sebagai pengganti. |
| C15 | **Laporan year-over-year** dan drill-down: klik kategori di laporan langsung ke daftar transaksinya. | Ada perbandingan periode; belum ada YoY dan drill-down. |
| C16 | **Rekonsiliasi impor dengan saldo rekening koran:** saldo akhir di file harus sama dengan saldo buku setelah impor; kalau beda, tunjukkan selisihnya. | Kolom saldo dibaca tapi tidak dicocokkan. |

### Pajak dan kewajiban Indonesia
| ID | Masukan | Sekarang |
|---|---|---|
| C17 | **SPT 1770 S yang lebih siap salin:** harta per kode harta DJP (kas, tabungan, piutang, investasi, kendaraan), utang per kode, penghasilan bruto, PPh 21 dipotong pemberi kerja. | Ringkasan SPT: harta, utang, penghasilan per akun. |
| C18 | **Zakat maal dan zakat profesi:** hitung nisab dan kewajiban dari harta yang sudah tercatat. | Tidak ada. |
| C19 | **THR dan bonus** ditandai supaya tidak membuat rata-rata pemasukan dan saving rate terlihat lebih tinggi dari rutin. | Tidak dibedakan. |

### Kerja harian pengguna mahir
| ID | Masukan | Sekarang |
|---|---|---|
| C20 | **Pintasan keyboard** di desktop (N untuk catat, / untuk cari, panah untuk periode) dan **edit massal** (ubah kategori banyak transaksi sekaligus, misalnya hasil impor). | Tidak ada. |
| C21 | **Akses data terprogram:** token API pribadi atau ekspor terjadwal untuk diolah di spreadsheet sendiri. | Ekspor ZIP manual. |

---

## 2. Persona D: pengguna awam

> "Pertama kali buka, saya senang: wizardnya gampang, catat cepatnya enak. Tapi setelah beberapa hari, saya mulai bingung. Menunya banyak sekali, ada kata-kata yang saya tidak tahu, dan saya sering lupa mencatat."

### Bingung di mana
| ID | Masukan | Sekarang |
|---|---|---|
| D1 | **Menunya terlalu banyak.** Di HP harus buka menu samping untuk semuanya. Saya cuma butuh 4–5 tombol: Ringkasan, Catat, Transaksi, Anggaran, Lainnya. | Menu samping (drawer) dengan 12 item + Lanjutan. |
| D2 | **Masih ada kata yang membuat saya takut:** "Debit/Kredit" di detail transaksi, "Ekuitas Saldo Awal", "Cocokkan Saldo", "Cek Keseimbangan", "Jurnal". | Banyak sudah diganti dan diberi ⓘ; beberapa sisa. |
| D3 | **Nama kategori campur Inggris:** "Cash", "Entertainment", "Admin", "E-wallet". Saya bingung "Admin" itu apa. | Nama template akun campuran. |
| D4 | **"Uang saya sekarang totalnya berapa?"** Angka itu yang paling saya cari, tapi tidak ada di paling atas. | Ada sisa aman per hari dan saldo per akun; total uang tidak menonjol. |
| D5 | **Kenapa periodenya tanggal 25?** Saya digaji mingguan (ojol) / tidak tentu (freelance). Saya ingin per bulan biasa saja. | Hanya periode gajian bulanan. |

### Lupa dan malas mencatat
| ID | Masukan | Sekarang |
|---|---|---|
| D6 | **Ingatkan saya setiap malam** untuk mencatat, lewat notifikasi HP. Dan tagihan yang mau jatuh tempo. | Hanya terlihat di dalam aplikasi. |
| D7 | **Catat dengan mengetik biasa:** "kopi 20rb gopay" atau "gaji 8jt" langsung jadi transaksi. | Keypad + pilih kategori. |
| D8 | **Foto struk saja**, nanti saya lengkapi. | Tidak ada. |
| D9 | **Catat lewat WhatsApp/Telegram** tanpa membuka aplikasi. | Tidak ada. |
| D10 | **Penyemangat:** berapa hari berturut-turut saya mencatat, dan pujian kecil kalau hemat. | Ada insight "hemat"; belum ada rangkaian hari. |

### Merasa aman dan nyaman
| ID | Masukan | Sekarang |
|---|---|---|
| D11 | **Sembunyikan angka** saat membuka aplikasi di depan orang (sekali tap, angka jadi ••••). | Tidak ada. |
| D12 | **Mode gelap** dan **huruf lebih besar**. | Tidak ada (mode gelap tertunda di 11b). |
| D13 | **Banner "email belum terverifikasi" mengganggu** dan saya tidak tahu kenapa harus. | Banner tampil terus sampai verifikasi. |
| D14 | **"Boleh beli ini nggak?"** Saya masukkan harga barang, aplikasi bilang aman atau tidak sampai gajian. | Tidak ada; bahannya (sisa aman per hari) sudah ada. |
| D15 | **Patungan dengan teman lebih gampang:** bagi rata ke 4 orang, lalu kirim tagihan WhatsApp ke masing-masing. | Talangan per kontak + tagih WA; belum ada bagi rata ke banyak orang. |
| D16 | **Kategori saya sendiri dari awal:** di wizard saya ingin memilih kategori yang relevan (anak, hewan, kendaraan) dan membuang yang tidak. | Kategori template tetap; tambah lewat Daftar Akun atau Anggaran. |

---

## 3. Sintesis

| Tema | Persona C | Persona D | Dampak |
|---|---|---|---|
| Navigasi mobile dan bahasa | – | D1, D2, D3, D4 | **Tinggi**: menentukan apakah orang awam bertahan setelah minggu pertama |
| Periode fleksibel | C10, C11 | D5 | **Tinggi**: banyak pengguna tidak cocok dengan "tanggal 1–28" |
| Kebiasaan mencatat | C20 | D6, D7, D8, D9, D10 | **Tinggi**: data yang tidak dicatat membuat semua laporan salah |
| Proyeksi dan "boleh beli?" | C12, C13, C14 | D14 | **Tinggi**: pertanyaan nomor satu kedua persona |
| Split, reimburse, patungan | C1, C3 | D15 | Sedang |
| Gaji, pajak, zakat Indonesia | C2, C17, C18, C19 | – | Sedang-tinggi untuk karyawan |
| Investasi, aset, kurs | C6, C7, C8, C9 | – | Sedang |
| Privasi dan kenyamanan | – | D11, D12, D13 | Sedang |
| Bukti, tag, data terprogram | C4, C5, C15, C16, C21 | D8 | Sedang |

Pola yang sama dengan putaran 1: persona awam butuh **lebih sedikit** (menu, kata, langkah), persona finance butuh **lebih dalam** (pajak, investasi, proyeksi). Keputusan "satu tampilan + ⓘ" tetap dipakai: hal mendalam ditaruh di Lanjutan dan detail transaksi, tidak di jalur utama.

---

## 4. Rencana fase

### Fase 12: Sederhana untuk Sehari-hari
**Menjawab:** D1–D5, D11–D13, D16, C11, dan mode gelap dari 11b.
- **Navigasi bawah di HP:** Ringkasan · Transaksi · [+ Catat] · Anggaran · Lainnya. "Lainnya" berisi semua menu lain dan Lanjutan. Desktop tetap sidebar.
- **Bersihkan istilah tersisa:** detail transaksi menampilkan "uang masuk/keluar dari" dulu, kolom Debit/Kredit di bawah ⓘ "lihat jurnal"; "Ekuitas Saldo Awal" ditampilkan sebagai "Kekayaan awal"; ganti nama menu "Cocokkan Saldo" → "Cek Saldo Dompet", "Cek Keseimbangan" tetap di Lanjutan.
- **Nama akun template berbahasa Indonesia** untuk pengguna baru (Tunai, Dompet Digital, Hiburan, Biaya Admin Bank); migration menawarkan ganti nama untuk pengguna lama yang belum mengubahnya.
- **"Total uangku"** sebagai angka paling atas di Ringkasan (kas + tabungan, dengan ⓘ apa yang dihitung).
- **Periode fleksibel:** gajian tanggal 1–31 atau "hari kerja terakhir", **bulan kalender**, atau **mingguan**. Perhitungan periode tetap satu fungsi (`internal/period`) dengan test tabel.
- **Sembunyikan angka** (ikon mata di header, disimpan per perangkat).
- **Mode gelap** (sapu ±80 warna terang yang ditulis langsung) dan **ukuran huruf** (normal/besar).
- **Wizard memilih kategori:** centang kategori yang relevan, sisanya dinonaktifkan.
- **Banner verifikasi** bisa ditutup 7 hari; alasan verifikasi dijelaskan dalam satu kalimat.
- **Selesai bila:** semua fitur utama terjangkau ≤ 2 tap dari layar mana pun di 390px; tidak ada kata Debit/Kredit/Ekuitas di jalur utama; periode tanggal 31 dan bulan kalender lulus test tabel (termasuk Februari).

### Fase 13: Kebiasaan Mencatat
**Menjawab:** D6–D10, C20.
- **Web Push** (VAPID): pengingat harian pada jam yang dipilih, tagihan kartu H-3, angsuran H-1, anggaran 80%/100%. Bisa dimatikan per jenis.
- **Ketik bebas di Catat cepat:** "kopi 20rb gopay", "gaji 8jt bca kemarin" → jumlah, kategori (aturan kata kunci + riwayat), dompet, tanggal. Parser deterministik dengan test tabel (tanpa layanan AI eksternal di fase ini).
- **Rangkaian hari mencatat** dan ringkasan mingguan singkat.
- **Pintasan keyboard** dan **edit massal** kategori.
- **Bot Telegram** (opsional, butuh keputusan): kirim teks yang sama dengan Catat cepat.
- **Selesai bila:** pengingat terkirim tepat waktu di HP yang memasang PWA; 30 contoh kalimat bebas terurai benar.

### Fase 14: Perencanaan dan Proyeksi
**Menjawab:** C12–C15, D14, dan anggaran amplop dari 11b.
- **Proyeksi 30/60/90 hari:** saldo kas + berulang + angsuran + tagihan kartu + sisa anggaran → grafik saldo harian dan tanggal saldo terendah.
- **"Boleh beli?"**: masukkan harga dan tanggal, jawab dengan dampaknya ke sisa aman per hari dan saldo terendah.
- **Kelompok kategori** (pokok/gaya hidup/tabungan) dengan rasio 50/30/20 di Kesehatan.
- **Anggaran amplop:** sisa terbawa per kategori (opsional), **biaya tahunan diratakan** (cadangan bulanan otomatis).
- **Laporan bulan kalender 12 kolom, YoY, dan drill-down** ke daftar transaksi.
- **Selesai bila:** proyeksi = saldo hari ini + semua kejadian terjadwal (diuji); klik angka kategori di laporan membuka transaksi yang menjumlah ke angka itu.

### Fase 15: Gaji, Pajak, dan Kewajiban Indonesia
**Menjawab:** C2, C3, C17–C19.
- **Template slip gaji:** bruto, PPh 21, BPJS (kesehatan, JHT, JP), potongan lain → netto ke rekening, dalam satu entry. Bisa dijadikan transaksi berulang.
- **Reimburse kantor:** kontak "Kantor" bawaan, tombol "minta reimburse" di detail transaksi.
- **SPT 1770 S:** harta per kode DJP (dipetakan dari kategori akun), utang, penghasilan bruto, PPh 21 dipotong; ekspor Excel siap salin.
- **Zakat maal dan profesi** dengan nisab yang diisi manual (harga emas per gram).
- **THR/bonus** ditandai "tidak rutin"; rata-rata dan saving rate rutin tidak memakainya.
- **Selesai bila:** penghasilan setahun di SPT = Σ bruto slip gaji (diuji); zakat dihitung sesuai contoh tabel.

### Fase 16: Investasi, Aset, dan Kurs
**Menjawab:** C6–C9, dan investasi/multi-mata uang dari 11b.
- **Instrumen investasi:** unit/lot/gram, harga per tanggal (diisi manual), nilai pasar, untung/rugi belum terealisasi dicatat ke akun ekuitas terpisah (bukan pendapatan rutin), dividen sebagai pendapatan.
- **Aset non-keuangan** dengan revaluasi atau penyusutan garis lurus.
- **Penanda likuiditas** per akun (kas, terkunci, tidak likuid) untuk dana darurat yang jujur.
- **Multi-mata uang:** akun dalam mata uang asing, kurs per transaksi (manual), selisih kurs.
- **Selesai bila:** neraca tetap seimbang setelah revaluasi; dana darurat tidak menghitung JHT/DPLK.

### Fase 17: Bukti, Tag, Patungan, dan Data
**Menjawab:** C1, C4, C5, C16, C21, D8, D15.
- **Split transaksi** di form biasa (beberapa kategori, satu sumber).
- **Tag** dan laporan per tag.
- **Lampiran struk/foto** (butuh keputusan penyimpanan), termasuk "foto dulu, lengkapi nanti".
- **Patungan ke banyak orang:** bagi rata atau per item, talangan ke tiap kontak, tagih WA per orang.
- **Cek saldo setelah impor** terhadap kolom saldo rekening koran.
- **Token API pribadi** (baca saja) untuk spreadsheet.
- **Selesai bila:** split dan patungan menghasilkan jurnal seimbang (diuji); saldo akhir impor yang beda ditandai dengan selisihnya.

---

## 5. Urutan dan alasannya

```
Fase 12  Sederhana untuk sehari-hari        <- orang awam bertahan setelah minggu pertama
Fase 13  Kebiasaan mencatat                  <- tanpa data yang dicatat, semua laporan salah
Fase 14  Perencanaan dan proyeksi            <- pertanyaan nomor satu kedua persona
Fase 15  Gaji, pajak, kewajiban Indonesia    <- karyawan bisa berhenti memakai Excel
Fase 16  Investasi, aset, kurs               <- kekayaan bersih lengkap
Fase 17  Bukti, tag, patungan, data          <- pelengkap untuk pengguna mahir
```

- Fase 12–13 dulu karena masalah terbesar sekarang bukan fitur yang kurang, tapi **orang berhenti mencatat**. Data yang bolong membuat proyeksi dan pajak tidak berguna.
- Fase 14 sebelum pajak dan investasi: semua pengguna merasakannya, dan bahannya (berulang, angsuran, tagihan, anggaran) sudah ada.
- Fase 16 setelah 15: penanda likuiditas dan aset dipakai juga oleh SPT (harta per kode).

## 6. Keputusan yang dibutuhkan

| Topik | Pilihan | Saran |
|---|---|---|
| Penyimpanan lampiran (Fase 17) | Disk server / S3-R2 | Disk server dulu di belakang antarmuka kecil, pindah ke S3/R2 saat di-deploy. |
| Push notification (Fase 13) | Web Push (PWA) saja / + email | Web Push dulu; email harian bisa mengganggu dan butuh domain Resend terverifikasi. |
| Bot chat (Fase 13) | Telegram / WhatsApp Business / tidak | Telegram (API gratis, mudah); WhatsApp Business berbayar dan butuh verifikasi bisnis. |
| Ketik bebas (Fase 13) | Parser aturan / model AI | Parser aturan dulu: deterministik, gratis, bisa diuji; AI opsional nanti. |
| Nama akun lama (Fase 12) | Ganti otomatis / tawarkan | Tawarkan sekali di Ringkasan; jangan ubah nama yang sudah diedit pengguna. |
| Periode mingguan (Fase 12) | Ya / hanya bulanan + kalender | Ya, karena pengemudi ojol dan pekerja harian termasuk sasaran. |

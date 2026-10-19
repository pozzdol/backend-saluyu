# Masukan Putaran 3 dan Rencana Fase 12–17 (revisi)

Ditulis 19 Okt 2026 setelah commit `9beab86` (frontend: audit UX, animasi, layout HP/tablet) dan `5b7a094`, `92dad98` (backend: email HTML, perbaikan hapus dan kunci, ganti nama grup, 5104 → "Hiburan").

Berbeda dari putaran 2, kali ini aplikasi **benar-benar dipakai** di browser:

- **Persona D (awam)** mendaftar akun baru di layar 390px, melewati wizard (BCA, GoPay, tunai; gajian tanggal 28 karena 30 tidak tersedia), mencatat jajan pertama lewat Catat cepat, lalu membuka Ringkasan, menu, detail transaksi, Cek Saldo, dan Daftar Akun, dalam English dan Indonesia.
- **Persona C (finance)** memakai akun berisi data (kartu kredit, kredit motor, impor) di desktop: Laporan, Kartu & Cicilan, Transaksi.

ID melanjutkan putaran 2 (C1–C21, D1–D16), jadi temuan baru mulai dari **C22** dan **D17**.

---

## 1. Yang sudah membaik sejak putaran 2

| Dari putaran 2 | Sekarang |
|---|---|
| D2 istilah menakutkan | Detail transaksi menampilkan alur uang ("BCA → Jajan"); jurnal debit/kredit disembunyikan di balik "Lihat jurnal". Tabel transaksi memakai kolom "Alur uang". **Sebagian selesai.** |
| D1 menu terlalu banyak | Menu dikelompokkan (Uang, Rencana, Orang, Laporan & cek, Pengaturan, Lanjutan). Masih menu samping, belum navigasi bawah. **Sebagian.** |
| D3 nama Inggris | "Entertainment" → "Hiburan" (migration 000018). "Cash", "Admin", "E-wallet" masih ada. **Sebagian.** |
| Mulai pakai | Kartu "Mulai di sini" dengan 4 langkah pertama di Ringkasan. **Baik.** |
| D15 patungan | Kontak punya "lunasi semua"; tagih WhatsApp sudah ada. Bagi rata ke banyak orang belum. **Sebagian.** |
| C15 drill-down | Baris laporan, buku besar, dan riwayat bisa diklik. **Sebagian** (YoY belum). |
| Lain-lain | Pratinjau angsuran bulanan, arsip dan ganti nama kartu, tautan kembali di halaman detail, pola notifikasi "Batalkan" yang seragam, tombol lihat password, layout tablet, email verifikasi dan reset berformat HTML. |

---

## 2. Temuan baru: persona D (awam)

> "Daftarnya cepat dan wizardnya jelas. Tapi begitu masuk, angka pertama yang saya lihat 'Aman dibelanjakan per hari: Rp 0', padahal saldo saya 3,4 juta. Setelah beli jajan 25 ribu, muncul 'Sisa uang −Rp 25.000, Defisit'. Rasanya saya sudah gagal di hari pertama."

| ID | Masukan | Bukti dari uji pakai |
|---|---|---|
| **D17** | **Hari pertama terasa gagal.** Gaji periode ini masuk sebelum buku dibuka, jadi pemasukan periode = 0: sisa aman Rp 0 dan "Defisit". Angka pertama harus jujur dan menenangkan: pakai uang yang ada (kas dikurangi tagihan terjadwal) dibagi sisa hari ketika periode belum punya pemasukan, dan jangan tampilkan "Defisit" di periode pembukaan. | Akun baru dengan saldo Rp 3.460.000 → sisa aman Rp 0; satu jajan Rp 25.000 → "Sisa uang −Rp 25.000 · Defisit di periode ini". |
| **D18** | **"Create your workspace"** di halaman daftar: "workspace" istilah kantor/SaaS. | Judul halaman daftar (English). |
| **D19** | **Kategori Catat cepat tidak cocok untuk sehari-hari.** "Bunga & Denda", "Beban Tunai Lain-lain", dan "Admin" muncul sejajar dengan Makan; sebaliknya kategori umum tidak ada (belanja bulanan, pulsa & internet, listrik & air, bensin, kesehatan, pendidikan, pakaian, sedekah). | Ubin kategori: Transportasi, Makan, Jajan, Hiburan, Keluarga, Admin, Bunga & Denda, Beban Tunai Lain-lain. |
| **D20** | **Tempat mengganti nama kategori justru paling teknis.** Daftar Akun menampilkan kode (5101), "bertambah di debit", "Ekuitas Saldo Awal", dan kelompok "Beban". Orang awam butuh halaman "Kategori" sederhana (nama, ikon, sembunyikan); daftar akun lengkap pindah ke Lanjutan. | Isi halaman Daftar Akun. |
| **D21** | **Periode tidak sesuai tanggal gajian sungguhan.** Gajian tanggal 30 dipaksa memilih 28, lalu periode tampil "28 Sep – 27 Okt". (Sama dengan C11, sekarang terbukti saat uji.) | Pilihan hari gajian hanya sampai 28. |
| **D22** | **Label teknis kecil masih bocor:** "Kas & Bank" di sebelah nama rekening, "Saldo harta", "Rasio menabung 0%". | Ringkasan versi Indonesia. |
| **D23** | **Akun baru dibanjiri kartu kosong:** kesehatan keuangan "Belum cukup data" tiga kali, tren 12 periode kosong, "Masuk tabungan" kosong. Tampilkan bertahap: sembunyikan kartu sampai datanya ada, dengan satu kalimat "akan muncul setelah X". | Ringkasan akun baru. |
| **D24** | **"Catat kebalikannya" di detail transaksi** membingungkan; orang awam akan menghapus. Pindahkan ke menu "lainnya" di detail, dengan penjelasan kapan dipakai (buku sudah ditutup). | Detail transaksi: Ubah · Catat kebalikannya · Hapus. |

Masukan putaran 2 yang **belum tersentuh** dan tetap relevan: D4 (total uangku di paling atas), D5 (periode bulanan/mingguan), D6–D10 (pengingat, ketik bebas, foto struk, bot, rangkaian hari), D11 (sembunyikan angka), D12 (mode gelap; belum ada pengaturan tema), D13 (banner verifikasi tidak bisa ditutup), D14 (boleh beli?), D16 (pilih kategori di wizard).

---

## 3. Temuan baru: persona C (finance)

> "Perbaikannya terasa: laporan bisa diklik sampai ke transaksi, angsuran punya pratinjau, kartu bisa diarsip. Yang mengganjal sekarang adalah bagaimana kejadian nyata dipetakan ke jurnal."

| ID | Masukan | Bukti dari uji pakai |
|---|---|---|
| **C22** | **Kredit kendaraan dicatat sebagai uang masuk ke bank.** Pada kredit motor, uang tidak pernah mampir di rekening; yang bertambah adalah aset kendaraan. Perlu akun aset tetap bawaan (Kendaraan, Properti, Elektronik) dan pilihan "pinjaman ini untuk membeli aset". | Jurnal: "Pencairan pinjaman · Kredit Motor → Bank Rp 18.000.000". |
| **C23** | **Satu akun "Bunga & Denda" untuk semuanya.** Bunga KPR/kredit, denda telat kartu, dan biaya bank tercampur, sehingga laporan tidak bisa menjawab "berapa biaya utang saya setahun". Pisahkan: bunga pinjaman, bunga & denda kartu, biaya bank/admin. | Laba rugi periode: "Bunga & Denda Rp 405.000" berisi bunga angsuran. |
| **C24** | **Daftar transaksi tanpa saldo berjalan dan tanpa ekspor hasil filter.** Untuk mencocokkan dengan rekening koran saya butuh kolom saldo per akun dan unduhan CSV/Excel dari filter yang sedang aktif (bukan hanya jurnal satu periode). | Halaman Transaksi: filter ada, ekspor hanya per periode di Laporan. |
| **C25** | **Buka-tutup buku tidak meninggalkan jejak.** Kalau kunci dibuka lalu transaksi lama diubah, tidak ada catatan siapa membuka kunci dan kapan. Untuk keluarga yang berbagi akses dan untuk audit pribadi, perlu log aktivitas pengaturan (kunci, saldo awal, hapus). | Revisi tersimpan per transaksi, tapi tidak untuk aksi buka kunci. |
| **C26** | **Saldo awal tampil sebagai "Ekuitas Saldo Awal → Bank"** di daftar transaksi biasa. Benar secara akuntansi, tapi di daftar harian sebaiknya "Saldo awal" saja, dengan nama ekuitas hanya di jurnal. | Baris pertama tabel Transaksi. |

Masukan putaran 2 yang **belum tersentuh**: C1 (split), C2 (slip gaji bruto-netto), C3 (reimburse), C4–C5 (tag, lampiran), C6 (kurs), C7–C9 (investasi, aset, likuiditas), C10–C11 (bulan kalender, gajian 29–31), C12 (proyeksi), C13–C14 (kelompok kategori, anggaran tahunan/amplop), C16 (cek saldo rekening koran setelah impor), C17–C19 (SPT per kode, zakat, THR), C20 (pintasan, edit massal), C21 (API).

---

## 4. Catatan untuk pengembang (bukan masukan produk)

- Commit `9beab86` menambah dependency `animejs`. Setelah `git pull`, jalankan `npm ci` di `frontend/`; tanpa itu halaman kosong tanpa error di konsol (Vite gagal me-resolve impor).
- Port 5173 di mesin ini sempat dipakai proyek lain (Laravel). `vite.config.ts` memakai `--strictPort` di skrip kita, jadi gunakan port lain (misalnya 5174) bila bentrok.

---

## 5. Rencana fase (revisi dari putaran 2)

Urutan fase tetap; isinya ditambah temuan putaran 3. Tanda **(baru)** menunjukkan tambahan.

### Fase 12: Sederhana untuk Sehari-hari
**Menjawab:** D1, D3–D5, D11–D13, D16, **D17–D24 (baru)**, C11, **C26 (baru)**.
- **(baru) Hari pertama yang jujur:** periode pembukaan tidak menampilkan "Defisit"; sisa aman per hari tanpa pemasukan periode memakai kas dikurangi tagihan/angsuran terjadwal sampai gajian; wizard menanyakan "gaji bulan ini sudah masuk?" agar pemasukan periode tidak nol.
- Navigasi bawah di HP (Ringkasan · Transaksi · + Catat · Anggaran · Lainnya).
- **(baru) Halaman "Kategori"** sederhana (nama, ikon, warna, sembunyikan, urutan); Daftar Akun lengkap pindah ke Lanjutan.
- **(baru) Kategori bawaan untuk sehari-hari:** belanja bulanan, pulsa & internet, listrik & air, bensin & parkir, kesehatan, pendidikan, pakaian, sedekah; kategori sistem (Bunga & Denda, Beban Tunai Lain-lain) tidak muncul di ubin Catat cepat kecuali pernah dipakai. Nama "Cash", "Admin", "E-wallet" → "Tunai", "Biaya Admin Bank", "Dompet Digital" untuk akun yang belum diubah.
- Periode: gajian 1–31 / hari kerja terakhir, bulan kalender, mingguan.
- "Total uangku" di paling atas; **(baru)** tanpa label kategori "Kas & Bank" di daftar saldo.
- **(baru) Ringkasan bertahap:** kartu tanpa data diganti satu baris "muncul setelah …".
- **(baru) Detail transaksi:** "Catat kebalikannya" pindah ke menu lainnya dengan penjelasan; saldo awal tampil sebagai "Saldo awal" di daftar harian.
- **(baru) "Create your workspace"** → "Buat akunmu".
- Sembunyikan angka, mode gelap + ukuran huruf, banner verifikasi yang bisa ditutup, pilih kategori di wizard.
- **Selesai bila:** akun baru dengan saldo > 0 dan tanpa pemasukan periode menampilkan sisa aman > 0 dan tidak ada kata "Defisit"; semua yang diuji di bagian 2 lolos ulang di 390px dalam Bahasa Indonesia.

### Fase 13: Kebiasaan Mencatat
Tidak berubah: Web Push (pengingat harian, tagihan, anggaran), ketik bebas, rangkaian hari, pintasan keyboard + edit massal, bot Telegram opsional.

### Fase 14: Perencanaan dan Proyeksi
Seperti putaran 2 (proyeksi 30/60/90, boleh beli, kelompok 50/30/20, anggaran amplop dan tahunan, laporan 12 bulan + YoY), **ditambah:**
- **(baru) C24:** kolom saldo berjalan saat jurnal difilter per akun, dan ekspor CSV/Excel dari filter yang aktif.

### Fase 15: Gaji, Pajak, dan Kewajiban Indonesia
Seperti putaran 2 (slip gaji, reimburse, SPT 1770 S, zakat, THR/bonus), **ditambah:**
- **(baru) C23:** akun bawaan terpisah untuk bunga pinjaman, bunga & denda kartu, dan biaya bank; migration memindahkan baris lama berdasarkan jenis entry (angsuran → bunga pinjaman, kartu → bunga & denda kartu).

### Fase 16: Investasi, Aset, dan Kurs
Seperti putaran 2, **ditambah:**
- **(baru) C22:** akun aset tetap bawaan (Kendaraan, Properti, Elektronik), dan pencairan cicilan bisa langsung ke aset ("kredit motor menambah Kendaraan, bukan Bank").

### Fase 17: Bukti, Tag, Patungan, dan Data
Seperti putaran 2, **ditambah:**
- **(baru) C25:** log aktivitas (buka/tutup kunci, ubah saldo awal, hapus transaksi, perubahan pengaturan) dengan siapa dan kapan, terlihat di Privasi & Data.

---

## 6. Keputusan yang dibutuhkan

Keputusan dari putaran 2 (penyimpanan lampiran, Web Push, bot, parser, nama akun lama, periode mingguan) masih terbuka. Tambahan dari putaran 3:

| Topik | Pilihan | Saran |
|---|---|---|
| Sisa aman hari pertama (D17) | Pakai kas yang ada / tanya gaji di wizard / keduanya | Keduanya: wizard bertanya, dan rumus berbasis kas sebagai cadangan. |
| Kategori sistem di Catat cepat (D19) | Sembunyikan / pindah ke bawah | Sembunyikan sampai pernah dipakai; tetap ada di form lengkap. |
| Memecah "Bunga & Denda" (C23) | Pindahkan riwayat lama / hanya untuk transaksi baru | Pindahkan otomatis untuk angsuran dan kartu (jenisnya jelas); yang lain tetap. |

---

## 7. Keputusan pengguna dan rencana final (19 Okt 2026)

Bagian ini **menggantikan** bagian 5 dan 6. Yang tidak dipilih masuk "Ditunda".

### Fase 12: Sederhana untuk Sehari-hari
- **Uang periode = sisa uang di awal periode + pemasukan periode**, berlaku di **setiap** periode (bukan hanya periode pertama). Sisa aman per hari dihitung dari uang periode dikurangi tagihan terjadwal sampai akhir periode. "Defisit" hanya muncul bila pengeluaran melebihi uang periode. Ini menjawab D17 tanpa pertanyaan tambahan di wizard.
- Periode: gajian tanggal 1–31 dan "hari kerja terakhir", bulan kalender, mingguan (C11, D5, D21).
- Kategori sistem (Bunga & Denda, Beban Tunai Lain-lain) disembunyikan dari ubin Catat cepat sampai pernah dipakai; tetap ada di form lengkap (D19).
- Nama akun lama berbahasa Inggris ("Cash", "Admin", "E-wallet"): banner "Ganti ke nama Indonesia?" dengan satu tombol; nama yang sudah diubah pengguna tidak disentuh (D3).
- "Total uangku" di paling atas; kartu tanpa data diganti satu baris "muncul setelah …" (D4, D23).
- Tombol sembunyikan angka (D11).
- Teks kecil: "Create your workspace" → "Buat akunmu", label "Kas & Bank" dihapus dari daftar saldo, "Catat kebalikannya" pindah ke menu lainnya dengan penjelasan, saldo awal tampil "Saldo awal" di daftar harian, banner verifikasi bisa ditutup, pilih kategori di wizard (D13, D16, D18, D22, D24, C26).
- Navigasi: **tetap menu samping** di HP.
- **Selesai bila:** akun baru dengan saldo > 0 dan tanpa pemasukan periode menampilkan sisa aman > 0 tanpa kata "Defisit"; gajian tanggal 30/31 bisa dipilih; Catat cepat akun baru tidak menampilkan kategori sistem.

### Fase 13: Pengingat Email
- Email lewat Resend yang sudah ada. Setiap jenis bisa diaktifkan/dimatikan sendiri di Pengaturan:
  - tagihan jatuh tempo (H-3 dan hari-H: berulang, cicilan, kartu);
  - anggaran hampir habis (80% dan 100%);
  - ringkasan mingguan (Senin);
  - lupa mencatat (3 hari tanpa transaksi).
- Rangkaian hari (streak) di Ringkasan.
- **Selesai bila:** tiap jenis email terkirim sekali per kejadian (tidak dobel saat job berjalan ulang) dan tidak terkirim bila dimatikan.

### Fase 14: Anggaran dan Ekspor
- Sisa anggaran terbawa ("amplop") bisa diatur per kategori; bawaan reset tiap periode.
- Kelompok 50/30/20 (Kebutuhan/Keinginan/Tabungan): kategori bawaan sudah dikelompokkan, pengguna bisa memindahkan.
- Ekspor CSV dan XLSX dari filter Transaksi yang aktif (C24).
- Kolom saldo berjalan saat Transaksi difilter satu akun (C24).

### Fase 15: Akun Lebih Tepat (gabungan Fase 15–16 lama)
- Pecah "Bunga & Denda" menjadi Bunga Pinjaman, Bunga & Denda Kartu, Biaya Bank; riwayat dari angsuran dan kartu dipindah otomatis lewat migration, sisanya tetap (C23).
- Akun aset tetap bawaan (Kendaraan, Properti, Elektronik); pencairan cicilan bisa langsung menambah aset, nilai diperbarui manual (C22).
- Fase 15 adalah fase terakhir.

### Ditunda (tidak dikerjakan sampai diminta)
- Mode gelap (D12), navigasi bawah (D1), halaman Kategori sederhana dan kategori harian tambahan (D19 sebagian, D20).
- Ketik bebas (D7), bot chat (D9), pintasan keyboard dan edit massal (C20), Web Push.
- Proyeksi 30/60/90, "boleh beli?", laporan 12 bulan + YoY, anggaran tahunan (C12–C15, D14).
- Slip gaji bruto, SPT 1770 S per kode, zakat, THR, reimburse (C2, C3, C17–C19). Gaji tetap dicatat netto.
- Investasi, multi mata uang, dana darurat (C6–C9).
- Log aktivitas, lampiran, tag, split, patungan bagi rata, cek saldo setelah impor, token API (C1, C4, C5, C16, C21, C25, D15).

# Masukan Pengguna dan Rencana Fase 6–11

Dua sudut pandang terhadap aplikasi setelah Fase 1–5 dan perbaikan tampilan mobile:

- **Persona A, praktisi finance.** Bekerja di bidang keuangan dan terbiasa mengatur keuangan sendiri dengan rapi.
- **Persona B, pengguna awam.** Tidak paham akuntansi, tapi ingin tahu ke mana uangnya pergi.

Setiap masukan diberi ID (A1, B1, …). Rencana fase di bagian akhir merujuk ke ID tersebut.

---

## 1. Persona A: praktisi finance

> "Fondasinya benar: double-entry, saldo awal, rekonsiliasi, periode gajian, piutang per orang. Itu sudah lebih disiplin daripada kebanyakan aplikasi pencatat keuangan. Tapi untuk dipakai setiap hari selama bertahun-tahun, masih ada celah besar."

### Pencatatan dan koreksi
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A1 | **Transaksi harus bisa diedit**, dengan jejak perubahan (siapa, kapan, nilai lama dan baru). Untuk periode yang sudah ditutup, koreksinya lewat jurnal pembalik, bukan menghapus. | Hanya bisa hapus lalu input ulang; tidak ada jejak. |
| A2 | **Tutup buku per periode.** Setelah rekonsiliasi, periode bisa dikunci sehingga transaksi lama tidak berubah diam-diam. | Semua periode selalu terbuka. |
| A3 | **Jurnal manual multi-baris** untuk penyesuaian, akrual, dan koreksi yang tidak muat di form sederhana. | Hanya form 2 baris dan form khusus. |
| A4 | **Transfer dengan biaya admin** dalam satu transaksi, misalnya Bank → E-wallet Rp100.000 dengan biaya Rp1.000. | Harus dicatat 2 transaksi. |
| A5 | **Tag lintas kategori** (misalnya "Liburan Bali 2026", "Renovasi") untuk melihat total biaya satu proyek. | Tidak ada. |
| A6 | **Lampiran bukti** (foto struk, invoice) per transaksi. | Tidak ada. |

### Utang konsumtif dan kartu kredit
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A7 | **Kartu kredit dan paylater sebagai sumber pembayaran.** Belanja memakai kartu kredit = beban bertambah dan utang kartu bertambah. | Aturan form pengeluaran hanya menerima akun Asset sebagai sumber, jadi kartu kredit tidak bisa dipakai sama sekali. |
| A8 | **Tagihan kartu:** tanggal cetak, jatuh tempo, minimum payment, dan pembayaran tagihan dari bank. | Tidak ada. |
| A9 | **Cicilan dan pinjaman berbunga** (KPR, kredit motor, KTA) dengan jadwal angsuran yang memisahkan pokok dan bunga. | Pinjaman ke kontak tanpa bunga saja. |

### Otomatisasi
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A10 | **Transaksi berulang:** gaji, sewa, langganan, cicilan, zakat. Bisa diposting otomatis atau jadi pengingat. | Semua manual. |
| A11 | **Impor mutasi** CSV/Excel dari bank (BCA, Mandiri, BRI, Seabank, Jago) dan e-wallet, dengan pemetaan kolom. | Tidak ada. |
| A12 | **Aturan kategorisasi otomatis**, misalnya keterangan mengandung "GRAB" masuk Transportasi. | Tidak ada. |
| A13 | **Rekonsiliasi per transaksi:** mencentang baris mutasi yang cocok, bukan hanya membandingkan saldo akhir. | Hanya membandingkan saldo dan membuat entry selisih. |

### Laporan dan analisis
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A14 | **Laporan arus kas** (operasi, investasi, pendanaan), karena laba rugi dan arus kas bisa sangat berbeda. | Ada Laba Rugi dan Neraca, tapi belum ada arus kas. |
| A15 | **Tren kekayaan bersih** dari bulan ke bulan, dan tren pengeluaran per kategori 6–12 periode. | Hanya satu periode per tampilan. |
| A16 | **Rasio kesehatan keuangan:** dana darurat (berapa bulan pengeluaran), rasio utang terhadap pendapatan, saving rate rata-rata 3/6/12 bulan, dan rasio pengeluaran wajib terhadap pengeluaran gaya hidup. | Hanya saving rate satu periode. |
| A17 | **Perbandingan antarperiode** (bulan ini vs bulan lalu vs rata-rata) di Laba Rugi dan budget. | Tidak ada. |
| A18 | **Ekspor Excel/PDF** untuk arsip dan dibagikan ke pasangan atau perencana keuangan. | Tidak ada. |
| A19 | **Ringkasan untuk SPT Tahunan:** daftar harta dan utang per 31 Desember, serta penghasilan setahun. Sangat relevan di Indonesia. | Tidak ada. |

### Perencanaan
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A20 | **Tujuan atau sinking fund:** dana darurat, DP rumah, pendidikan. Ada target, tenggat, kebutuhan setoran per bulan, dan progres. | Tidak ada. |
| A21 | **Sisa budget bisa terbawa** ke periode berikutnya (amplop), dan budget per kelompok kategori. | Budget terbawa, tapi sisa uangnya tidak. |
| A22 | **Peringatan budget** di 80% dan 100%. | Hanya bar merah saat sudah lewat. |
| A23 | **Biaya tahunan yang diratakan** (asuransi, pajak kendaraan) supaya bulan pembayaran tidak terlihat defisit palsu. | Tidak ada. |
| A24 | **Nilai investasi saat ini** diisi manual, untung/rugi belum terealisasi, beberapa instrumen, dividen. Keputusan awal "harga beli saja" perlu ditinjau ulang setelah aplikasi stabil. | Harga beli saja. |
| A25 | **Multi-mata uang** (tabungan USD, perjalanan ke luar negeri). | IDR saja. |

### Keamanan dan data
| ID | Masukan | Kondisi sekarang |
|---|---|---|
| A26 | **Lupa password dan verifikasi email**, serta **2FA** untuk data sesensitif ini. | Tidak ada reset password. |
| A27 | **Daftar sesi aktif** dan "logout dari semua perangkat". | Hanya terjadi otomatis saat ganti password. |
| A28 | **Backup dan ekspor penuh** (JSON/CSV) supaya data tidak terkunci di aplikasi. | Tidak ada. |

---

## 2. Persona B: pengguna awam

> "Saya cuma ingin tahu uang saya habis ke mana dan masih cukup atau tidak sampai gajian. Begitu masuk, saya ketemu kata Debit, Credit, Ledger, Trial balance, Equity... saya takut salah pencet."

### Bahasa dan istilah
| ID | Masukan |
|---|---|
| B1 | **Pakai Bahasa Indonesia.** Sekarang semua teks berbahasa Inggris. |
| B2 | **Istilah akuntansi bikin takut.** Ganti dengan bahasa sehari-hari: "Dompet/Rekening" (bukan Asset), "Kategori" (bukan akun Expense), "Uang masuk/keluar" (bukan Debit/Credit), "Utang/Piutang ke teman". Kode akun (1001, 5102) sebaiknya disembunyikan. |
| B3 | **Halaman teknis disembunyikan** (Trial balance, Ledger, Opening balance, Accounts) di menu "Lanjutan". Menu yang terlalu banyak membingungkan. |
| B4 | **"Reconcile" dan "Cash count" tidak dimengerti.** Ganti jadi "Cocokkan saldo" dengan penjelasan satu kalimat. |

### Mulai pakai
| ID | Masukan |
|---|---|
| B5 | **Halaman pertama langsung meminta "saldo awal aset dan liabilitas".** Lebih enak berupa wizard tanya-jawab: "Kamu punya rekening apa saja?" (pilih logo bank/e-wallet), "Berapa saldonya sekarang?", "Tanggal berapa gajian?" |
| B6 | **Contoh dan panduan saat halaman masih kosong**, misalnya "Catat pengeluaran pertamamu: kopi pagi ini?". |

### Mencatat harus secepat mungkin
| ID | Masukan |
|---|---|
| B7 | **Tombol "+" besar yang selalu ada di HP.** Alurnya: ketik jumlah → pilih kategori (ikon besar) → simpan. Tiga langkah, tanggal otomatis hari ini, dompet otomatis yang terakhir dipakai. |
| B8 | **Kategori dengan ikon dan warna** (🍜 Makan, 🚗 Transport, ☕ Jajan), plus kategori yang sering dipakai muncul paling atas. |
| B9 | **Keypad angka sendiri** dengan tombol "000" dan format rupiah langsung saat mengetik. |
| B10 | **Salah ketik harus bisa diedit**, bukan hapus lalu buat ulang. Setelah hapus, ada tombol "Batalkan" beberapa detik. |
| B11 | **Cari transaksi berdasarkan jumlah**, misalnya "75.000". |

### Memahami kondisi uang
| ID | Masukan |
|---|---|
| B12 | **Grafik "uang habis ke mana"** (donat per kategori) dan grafik harian. |
| B13 | **"Sisa aman dibelanjakan per hari sampai gajian"**, satu angka besar di dashboard. |
| B14 | **Insight sederhana**, misalnya "Jajan bulan ini 30% lebih banyak dari bulan lalu" atau "Hore, kamu hemat Rp200.000!". |
| B15 | **Budget yang gampang:** cukup isi batas per kategori dan dapat notifikasi kalau hampir habis. |

### Teman, keluarga, dan pengingat
| ID | Masukan |
|---|---|
| B16 | **Tagih teman lewat WhatsApp:** tombol "Ingatkan" yang membuat pesan "Halo Bau, sisa talangan makan Rp60.000 ya 🙏". |
| B17 | **Undang ke grup lewat link atau WhatsApp**, bukan cuma email. |
| B18 | **Pengingat harian** untuk mencatat, dan pengingat tagihan (listrik, internet, cicilan). |

### Kenyamanan dan rasa aman
| ID | Masukan |
|---|---|
| B19 | **Bisa dipasang di layar HP seperti aplikasi (PWA)**, cepat dibuka, dan tetap bisa mencatat saat sinyal jelek. |
| B20 | **Mode gelap.** |
| B21 | **Lupa password harus bisa reset sendiri.** Login dengan Google juga lebih praktis. |
| B22 | **Kepastian data aman:** bisa unduh semua data dan ada penjelasan siapa saja yang bisa melihat. |

---

## 3. Sintesis: apa yang paling penting

Masukan yang muncul dari **kedua persona** diberi bobot tertinggi:

| Tema | Persona A | Persona B | Prioritas |
|---|---|---|---|
| Edit transaksi + jejak + batalkan hapus | A1 | B10 | **Sangat tinggi** |
| Lupa password, keamanan akun | A26, A27 | B21 | **Sangat tinggi** (fitur dasar yang belum ada) |
| Bahasa Indonesia + mode sederhana | – | B1–B4 | **Sangat tinggi**: penentu apakah pengguna awam mau bertahan |
| Pencatatan cepat + kategori berikon | A5 | B7–B9 | **Tinggi** |
| Transaksi berulang + pengingat | A10 | B18 | **Tinggi** |
| Grafik, tren, insight | A15–A17 | B12–B14 | **Tinggi** |
| Kartu kredit / paylater / cicilan | A7–A9 | – | **Tinggi**: tanpa ini, banyak pengeluaran nyata tidak bisa dicatat dengan benar |
| Impor mutasi + aturan kategori | A11–A13 | – | Sedang–tinggi |
| Tujuan / sinking fund | A20, A23 | B13 | Sedang |
| Ekspor, backup, SPT | A18, A19, A28 | B22 | Sedang |
| PWA, offline, mode gelap | – | B19, B20 | Sedang |
| Investasi nilai pasar, multi-mata uang | A24, A25 | – | Rendah (setelah inti stabil) |

Prinsip yang dipegang (diputuskan, lihat bagian 6): **satu tampilan untuk semua.** Struktur halaman tetap mengikuti alur finance (jurnal, buku besar, neraca saldo, laporan), tetapi judul dan label memakai bahasa sehari-hari. Setiap istilah punya ikon ⓘ yang menjelaskan maksudnya dan menyebut istilah resminya. Praktisi tetap menemukan semua yang ia butuhkan, dan pengguna awam tidak tersesat.

---

## 4. Rencana fase lanjutan

Urutan disusun supaya setiap fase bisa dipakai sendiri, dan fase yang menambah pengguna baru (bahasa, kemudahan) datang sebelum fase yang menambah kedalaman.

### Fase 6: Bahasa, Istilah Sehari-hari + Penjelasan ⓘ, Pencatatan Cepat, dan Edit
**Menjawab:** B1–B4, B6–B10, A1, A5 (dasar).
- **i18n Indonesia + English.** Browser berbahasa Indonesia (`navigator.language` diawali `id`) mendapat Bahasa Indonesia, selain itu English. Pilihan bisa diubah di Profil dan disimpan di `users.locale`. Kamus teks di frontend; format tanggal dan angka mengikuti locale.
- **Istilah sehari-hari + ⓘ** (satu tampilan, tanpa mode ganda). Contoh: "Neraca Saldo" jadi "Cek Keseimbangan", "Buku Besar" jadi "Riwayat Akun", "Rekonsiliasi" jadi "Cocokkan Saldo", "Hitung kas" jadi "Cocokkan Uang Tunai", "Laba Rugi" jadi "Pemasukan & Pengeluaran". Setiap judul, label kolom, dan istilah penting punya ikon ⓘ:
  - **desktop:** muncul saat hover atau fokus keyboard;
  - **HP:** tap membuka **bottom sheet** berisi penjelasan, contoh angka, dan istilah resminya;
  - isi penjelasan disimpan di satu kamus (`glossary`) per bahasa supaya konsisten di semua halaman. Kode akun tetap ada, tetapi tampil kecil dan sekunder.
- **Kategori berikon dan berwarna:** kolom `accounts.icon` dan `accounts.color`, diisi untuk template default. Kategori yang sering dipakai muncul paling atas (dihitung dari jurnal 30 hari terakhir).
- **Quick Add:** tombol "+" mengambang di HP. Bottom sheet berisi keypad angka (dengan tombol 000), lalu grid kategori berikon, lalu simpan. Dompet terakhir diingat per user.
- **Edit transaksi, semua jenis**, lewat jalur validasi yang sama dengan `post()`. Setiap perubahan dicatat di tabel baru `entry_revisions` (snapshot lama dan baru, waktu, user) dan bisa dilihat sebagai "Riwayat perubahan".
  - **income / expense / transfer / cash_count / adjustment:** edit langsung.
  - **opening:** lewat halaman saldo awal; boleh diedit walaupun buku sudah berisi transaksi lain (aturan "hanya saat buku kosong" dicabut, karena kini ada riwayat).
  - **loan / repayment tanpa kontak tertaut, atau dengan permintaan yang belum di-approve:** edit langsung; permintaan pending ikut diperbarui.
  - **loan / repayment yang sudah di-approve:** edit menjadi **permintaan perubahan** ke pihak lain (tabel `loan_change_requests`). Setelah disetujui, kedua buku berubah dalam satu DB transaction. Sebelum disetujui, nilai lama tetap berlaku. Kalau ditolak, tidak ada yang berubah.
- **Batalkan hapus:** hapus tidak langsung permanen. Entry ditandai `deleted_at`, dan ada toast "Batalkan" selama 8 detik. Pembersihan permanen dijalankan berkala.
- **Selesai bila:** semua teks tersedia dalam bahasa Indonesia dan English; setiap istilah finance punya ⓘ yang bisa dibuka dengan hover, keyboard, dan tap; pengguna baru bisa mencatat pengeluaran dalam 3 tap dari dashboard; mengedit transaksi jenis apa pun memperbarui laporan dan meninggalkan riwayat; edit talangan yang sudah di-approve baru mengubah kedua buku setelah pihak lain setuju.

### Fase 7: Keamanan Akun dan Onboarding Ramah
**Menjawab:** A26, A27, B5, B21, B22.
- **Lupa password:** token sekali pakai (di-hash, kedaluwarsa 30 menit) dikirim lewat **Resend API** dari `noreply@fikriachmad.dev`. Domain `fikriachmad.dev` perlu diverifikasi di Resend (record SPF/DKIM yang kamu pasang di DNS). API key disimpan di `.env` (`RESEND_API_KEY`), tidak di-commit. Saat development tanpa API key, link reset dicetak di log server. Setelah reset, semua sesi dicabut. Respons "email terkirim" selalu sama, terdaftar atau tidak, supaya tidak membocorkan siapa yang punya akun.
- **Verifikasi email**, yang juga melengkapi alur undangan grup per email.
- **Daftar sesi aktif** (perangkat, terakhir aktif), tombol cabut per sesi, dan "logout semua".
- **2FA TOTP** (opsional per user) dengan kode cadangan.
- **Wizard onboarding** (pengganti tampilan awal halaman saldo awal untuk pengguna baru): pilih bank/e-wallet dari daftar berlogo, isi saldo, pilih tanggal gajian, lalu (opsional) isi 3 budget pertama.
- **Halaman "Privasi & Data":** penjelasan siapa melihat apa di grup, dan unduh semua data (JSON + CSV per tabel).
- **Selesai bila:** user bisa reset password sendiri; onboarding bisa diselesaikan tanpa melihat kata "aset" atau "liabilitas".

### Fase 8: Kartu Kredit, Paylater, dan Cicilan
**Menjawab:** A4, A7, A8, A9.
- **Sumber pembayaran boleh berupa Liability.** Aturan `expense` diperluas jadi (Asset atau Liability) → Expense. Transaksi kartu kredit: Dr Beban / Cr Utang Kartu.
- **Akun kartu** (`accounts.kind = credit_card`, plus tanggal cetak dan jatuh tempo). Halaman tagihan menampilkan transaksi per siklus, total tagihan, minimum, dan tombol "Bayar tagihan" (transfer Bank → Utang Kartu).
- **Cicilan berbunga:** tabel `installment_plans` (pokok, bunga, tenor, metode flat/anuitas) menghasilkan jadwal. Setiap pembayaran dipecah jadi Dr Utang (pokok) + Dr Beban Bunga / Cr Bank.
- **Transfer dengan biaya** dalam satu entry: Dr Tujuan + Dr Beban Admin / Cr Sumber.
- **Selesai bila:** belanja dengan kartu kredit muncul di pengeluaran periode belanja (bukan saat bayar tagihan), utang kartu tampil di neraca, dan jadwal cicilan KPR menghitung bunga dengan benar (diuji dengan tabel test).

### Fase 9: Otomatisasi, yaitu Transaksi Berulang, Pengingat, dan Impor Mutasi
**Menjawab:** A10–A13, B18.
- **Transaksi berulang:** tabel `recurring_rules` (pola harian/mingguan/bulanan/tanggal gajian, mode auto-post atau pengingat). Job harian di Go (goroutine + ticker, atau cron eksternal) memposting lewat `post()` secara idempoten, dengan kunci unik (rule, tanggal).
- **Pengingat dan notifikasi:** in-app (memakai inbox yang sudah ada), email, dan Web Push setelah PWA (Fase 11).
- **Impor CSV/XLSX mutasi:** unggah file, petakan kolom (tersimpan per bank), pratinjau, deteksi duplikat (hash tanggal + jumlah + keterangan), lalu posting massal dalam satu DB transaction.
- **Aturan kategorisasi** (`categorization_rules`: kata kunci, rentang jumlah, ke kategori) diterapkan saat impor dan saat quick add.
- **Rekonsiliasi per transaksi:** baris mutasi dicocokkan dengan entry yang ada; yang tidak cocok ditawarkan untuk dibuat.
- **Selesai bila:** gaji tanggal 25 terposting otomatis tepat sekali walau job dijalankan dua kali, dan impor 200 baris mutasi BCA tidak membuat duplikat saat diimpor ulang.

### Fase 10: Insight, Grafik, dan Laporan Lengkap
**Menjawab:** A14–A19, A22, B11–B15.
- **Dashboard berlapis:** di atas, angka besar "Sisa aman per hari sampai gajian" (sisa budget dibagi sisa hari), donat per kategori, grafik harian, dan 3 insight otomatis (naik/turun dibanding periode lalu dan rata-rata 3 periode). Di bawahnya, tren kekayaan bersih 12 periode dan tren per kategori, masing-masing dengan ⓘ.
- **Rasio kesehatan:** dana darurat (aset lancar ÷ rata-rata pengeluaran bulanan), rasio utang terhadap pendapatan, saving rate rata-rata 3/6/12. Masing-masing dengan penjelasan satu kalimat dan ambang warna.
- **Laporan Arus Kas** (metode langsung, karena setiap entry sudah tahu akun kas yang terlibat) dan perbandingan antarperiode di Laba Rugi.
- **Peringatan budget** 80% dan 100% (in-app, plus push setelah Fase 11).
- **Ekspor Excel dan PDF** untuk Laba Rugi, Neraca, Arus Kas, dan jurnal.
- **Ringkasan SPT Tahunan:** daftar harta dan utang per 31 Desember dan total penghasilan setahun, dengan catatan bahwa ini bantuan rekap, bukan nasihat pajak.
- **Pencarian jurnal berdasarkan jumlah dan rentang jumlah.**
- **Selesai bila:** semua grafik mengikuti aturan aksesibilitas (tidak bergantung pada warna saja, ada tabel alternatif), angka di grafik sama dengan laporan, dan arus kas bersih sama dengan perubahan saldo kas di neraca (diuji).

### Fase 11: Tujuan, Perencanaan, dan Platform
**Menjawab:** A2, A3, A6, A20, A21, A23, A24, A25, A28, B16, B17, B19, B20.
- **Tujuan / sinking fund:** target, tenggat, dan akun penampung. Setoran per bulan yang dibutuhkan dihitung otomatis; progres tampil di dashboard.
- **Budget amplop:** sisa budget bisa dibawa (opsional per kategori). Biaya tahunan bisa diratakan lewat cadangan bulanan.
- **Tutup buku per periode** (`period_locks`) dan **jurnal manual multi-baris** (di menu Lanjutan, dengan ⓘ). Koreksi di periode terkunci memakai jurnal pembalik.
- **Lampiran struk** (penyimpanan objek, di-resize di server) dan **tag** lintas kategori.
- **Investasi lanjutan:** nilai pasar diisi manual per tanggal, untung/rugi belum terealisasi sebagai pendapatan komprehensif terpisah, dan dividen. **Multi-mata uang** (kurs manual per transaksi).
- **PWA:** manifest, ikon, service worker untuk app shell, antrean offline untuk quick add, dan **mode gelap** (token warna sudah tersedia di CSS).
- **Berbagi lewat WhatsApp:** pesan tagih utang dan link undangan grup bertoken sekali pakai.
- **Selesai bila:** aplikasi bisa dipasang di layar HP, quick add yang dibuat saat offline tersinkron tanpa dobel, dan periode yang dikunci menolak perubahan dengan pesan yang jelas.

---

## 5. Urutan dan alasannya

```
Fase 6  Bahasa + istilah sehari-hari ⓘ + Quick Add + Edit  <- membuka pintu untuk pengguna awam
Fase 7  Keamanan akun + Onboarding ramah             <- celah dasar (lupa password) ditutup
Fase 8  Kartu kredit, paylater, cicilan              <- pengeluaran nyata bisa dicatat benar
Fase 9  Berulang, pengingat, impor mutasi            <- mengurangi kerja manual harian
Fase 10 Insight, grafik, laporan lengkap             <- data sudah cukup kaya untuk dianalisis
Fase 11 Tujuan, platform, fitur lanjutan             <- penyempurnaan jangka panjang
```

- Fase 6 dan 7 menentukan apakah pengguna awam **mau memakai** aplikasinya. Fase 8 dan 9 menentukan apakah datanya **lengkap dan akurat**. Fase 10 baru bernilai setelah datanya lengkap.
- Fase 8 sengaja ditaruh sebelum otomatisasi: impor mutasi kartu kredit butuh akun kartu yang benar.
- Setiap fase akan diturunkan menjadi dokumen rinci (`fase-6.md`, dst.) dengan format yang sama seperti fase 1–5: migration, endpoint, aturan debit/kredit, frontend, test, dan langkah kerja.

## 6. Keputusan diskusi (sebelum Fase 6)
| Topik | Keputusan |
|---|---|
| Bahasa default | Browser berbahasa Indonesia mendapat **Bahasa Indonesia**, selain itu **English**. Bisa diubah di Profil. |
| Mode tampilan | **Satu tampilan.** Struktur halaman tetap gaya finance, bahasa disederhanakan, setiap istilah punya penjelasan **ⓘ**. Tidak ada Mode Sederhana/Lengkap. |
| Istilah di judul dan menu | **Istilah sehari-hari + ⓘ.** Istilah resmi disebut di dalam penjelasan. |
| ⓘ di HP | Tap membuka **bottom sheet** (penjelasan, contoh, istilah resmi). Di desktop: hover atau fokus. |
| Cakupan edit transaksi | **Semua jenis**, dengan riwayat perubahan. |
| Edit talangan yang sudah di-approve | **Permintaan perubahan** ke pihak lain. Kedua buku berubah bersamaan setelah disetujui. |
| Layanan email | **Resend API**, pengirim `noreply@fikriachmad.dev` (domain perlu diverifikasi di Resend). |
| Transaksi berulang (Fase 9) | **Pengingat dulu** ("siap dicatat", tinggal konfirmasi); **auto-post opsional** per aturan untuk yang jumlahnya pasti. |
| Impor mutasi (Fase 9) | Didukung semua: **BCA, bank digital (Jago, Seabank, Blu), Mandiri/BRI/BNI, e-wallet**. Pemetaan kolom CSV/XLSX generik dengan preset per bank; e-wallet memakai pemetaan manual karena ekspornya terbatas. |
| Kartu kredit (Fase 8) | **Siklus tagihan lengkap:** tanggal cetak, jatuh tempo, total tagihan, minimum payment, bayar tagihan, peringatan jatuh tempo. Paylater memakai mekanisme yang sama. |
| Langkah berikutnya | Rinci Fase 6 (`fase-6.md`), lalu kerjakan. |

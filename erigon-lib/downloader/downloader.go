/*
   Copyright 2021 Erigon contributors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
	"github.com/c2h5oh/datasize"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/datadir"
	"github.com/ledgerwatch/erigon-lib/common/dbg"
	"github.com/ledgerwatch/erigon-lib/common/dir"
	"github.com/ledgerwatch/erigon-lib/diagnostics"
	"github.com/ledgerwatch/erigon-lib/downloader/downloadercfg"
	"github.com/ledgerwatch/erigon-lib/downloader/snaptype"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/log/v3"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// Downloader - component which downloading historical files. Can use BitTorrent, or other protocols
type Downloader struct {
	db                kv.RwDB
	pieceCompletionDB storage.PieceCompletion
	torrentClient     *torrent.Client

	cfg *downloadercfg.Cfg

	statsLock *sync.RWMutex
	stats     AggStats

	folder storage.ClientImplCloser

	ctx          context.Context
	stopMainLoop context.CancelFunc
	wg           sync.WaitGroup

	webseeds  *WebSeeds
	logger    log.Logger
	verbosity log.Lvl
}

type AggStats struct {
	MetadataReady, FilesTotal int32
	PeersUnique               int32
	ConnectionsTotal          uint64

	Completed bool
	Progress  float32

	BytesCompleted, BytesTotal     uint64
	DroppedCompleted, DroppedTotal uint64

	BytesDownload, BytesUpload uint64
	UploadRate, DownloadRate   uint64
}

func New(ctx context.Context, cfg *downloadercfg.Cfg, dirs datadir.Dirs, logger log.Logger, verbosity log.Lvl, discover bool) (*Downloader, error) {
	db, c, m, torrentClient, err := openClient(ctx, cfg.Dirs.Downloader, cfg.Dirs.Snap, cfg.ClientConfig)
	if err != nil {
		return nil, fmt.Errorf("openClient: %w", err)
	}

	peerID, err := readPeerID(db)
	if err != nil {
		return nil, fmt.Errorf("get peer id: %w", err)
	}
	cfg.ClientConfig.PeerID = string(peerID)
	if len(peerID) == 0 {
		if err = savePeerID(db, torrentClient.PeerID()); err != nil {
			return nil, fmt.Errorf("save peer id: %w", err)
		}
	}

	d := &Downloader{
		cfg:               cfg,
		db:                db,
		pieceCompletionDB: c,
		folder:            m,
		torrentClient:     torrentClient,
		statsLock:         &sync.RWMutex{},
		webseeds:          &WebSeeds{logger: logger, verbosity: verbosity, downloadTorrentFile: cfg.DownloadTorrentFilesFromWebseed, torrentsWhitelist: cfg.ExpectedTorrentFilesHashes},
		logger:            logger,
		verbosity:         verbosity,
	}
	d.ctx, d.stopMainLoop = context.WithCancel(ctx)

	if err := d.BuildTorrentFilesIfNeed(d.ctx); err != nil {
		return nil, err
	}
	if err := d.addTorrentFilesFromDisk(false); err != nil {
		return nil, err
	}

	// CornerCase: no peers -> no anoncments to trackers -> no magnetlink resolution (but magnetlink has filename)
	// means we can start adding weebseeds without waiting for `<-t.GotInfo()`
	d.wg.Add(1)

	go func() {
		defer d.wg.Done()
		if !discover {
			return
		}
		d.webseeds.Discover(d.ctx, d.cfg.WebSeedS3Tokens, d.cfg.WebSeedUrls, d.cfg.WebSeedFiles, d.cfg.Dirs.Snap)
		// webseeds.Discover may create new .torrent files on disk
		if err := d.addTorrentFilesFromDisk(true); err != nil && !errors.Is(err, context.Canceled) {
			d.logger.Warn("[snapshots] addTorrentFilesFromDisk", "err", err)
		}
	}()
	return d, nil
}

const prohibitNewDownloadsFileName = "prohibit_new_downloads.lock"

// Erigon "download once" - means restart/upgrade/downgrade will not download files (and will be fast)
// After "download once" - Erigon will produce and seed new files
// Downloader will able: seed new files (already existing on FS), download uncomplete parts of existing files (if Verify found some bad parts)
func (d *Downloader) prohibitNewDownloads() error {
	fPath := filepath.Join(d.SnapDir(), prohibitNewDownloadsFileName)
	f, err := os.Create(fPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Sync(); err != nil {
		return err
	}
	return nil
}
func (d *Downloader) newDownloadsAreProhibited() bool {
	return dir.FileExist(filepath.Join(d.SnapDir(), prohibitNewDownloadsFileName))
}

func (d *Downloader) MainLoopInBackground(silent bool) {
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		if err := d.mainLoop(silent); err != nil {
			if !errors.Is(err, context.Canceled) {
				d.logger.Warn("[snapshots]", "err", err)
			}
		}
	}()
}

func (d *Downloader) mainLoop(silent bool) error {
	var sem = semaphore.NewWeighted(int64(d.cfg.DownloadSlots))

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		// Torrents that are already taken care of
		//// First loop drops torrents that were downloaded or are already complete
		//// This improves efficiency of download by reducing number of active torrent (empirical observation)
		//for torrents := d.torrentClient.Torrents(); len(torrents) > 0; torrents = d.torrentClient.Torrents() {
		//	select {
		//	case <-d.ctx.Done():
		//		return
		//	default:
		//	}
		//	for _, t := range torrents {
		//		if _, already := torrentMap[t.InfoHash()]; already {
		//			continue
		//		}
		//		select {
		//		case <-d.ctx.Done():
		//			return
		//		case <-t.GotInfo():
		//		}
		//		if t.Complete.Bool() {
		//			atomic.AddUint64(&d.stats.DroppedCompleted, uint64(t.BytesCompleted()))
		//			atomic.AddUint64(&d.stats.DroppedTotal, uint64(t.Length()))
		//			t.Drop()
		//			torrentMap[t.InfoHash()] = struct{}{}
		//			continue
		//		}
		//		if err := sem.Acquire(d.ctx, 1); err != nil {
		//			return
		//		}
		//		t.AllowDataDownload()
		//		t.DownloadAll()
		//		torrentMap[t.InfoHash()] = struct{}{}
		//		d.wg.Add(1)
		//		go func(t *torrent.Torrent) {
		//			defer d.wg.Done()
		//			defer sem.Release(1)
		//			select {
		//			case <-d.ctx.Done():
		//				return
		//			case <-t.Complete.On():
		//			}
		//			atomic.AddUint64(&d.stats.DroppedCompleted, uint64(t.BytesCompleted()))
		//			atomic.AddUint64(&d.stats.DroppedTotal, uint64(t.Length()))
		//			t.Drop()
		//		}(t)
		//	}
		//}
		//atomic.StoreUint64(&d.stats.DroppedCompleted, 0)
		//atomic.StoreUint64(&d.stats.DroppedTotal, 0)
		//d.addTorrentFilesFromDisk(false)
		for {
			torrents := d.torrentClient.Torrents()
			select {
			case <-d.ctx.Done():
				return
			default:
			}
			for _, t := range torrents {
				if t.Complete.Bool() {
					continue
				}
				if err := sem.Acquire(d.ctx, 1); err != nil {
					return
				}
				t.AllowDataDownload()
				select {
				case <-d.ctx.Done():
					return
				case <-t.GotInfo():
				}
				t.DownloadAll()
				d.wg.Add(1)
				go func(t *torrent.Torrent) {
					defer d.wg.Done()
					defer sem.Release(1)
					select {
					case <-d.ctx.Done():
						return
					case <-t.Complete.On():
					}
				}(t)
			}

			select {
			case <-d.ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
		}
	}()

	logEvery := time.NewTicker(20 * time.Second)
	defer logEvery.Stop()

	statInterval := 20 * time.Second
	statEvery := time.NewTicker(statInterval)
	defer statEvery.Stop()

	var m runtime.MemStats
	justCompleted := true
	for {
		select {
		case <-d.ctx.Done():
			return d.ctx.Err()
		case <-statEvery.C:
			d.ReCalcStats(statInterval)

		case <-logEvery.C:
			if silent {
				continue
			}

			stats := d.Stats()

			dbg.ReadMemStats(&m)
			if stats.Completed {
				if justCompleted {
					justCompleted = false
					// force fsync of db. to not loose results of downloading on power-off
					_ = d.db.Update(d.ctx, func(tx kv.RwTx) error { return nil })
				}

				d.logger.Info("[snapshots] Seeding",
					"up", common.ByteCount(stats.UploadRate)+"/s",
					"peers", stats.PeersUnique,
					"conns", stats.ConnectionsTotal,
					"files", stats.FilesTotal,
					"alloc", common.ByteCount(m.Alloc), "sys", common.ByteCount(m.Sys),
				)
				continue
			}

			d.logger.Info("[snapshots] Downloading",
				"progress", fmt.Sprintf("%.2f%% %s/%s", stats.Progress, common.ByteCount(stats.BytesCompleted), common.ByteCount(stats.BytesTotal)),
				"download", common.ByteCount(stats.DownloadRate)+"/s",
				"upload", common.ByteCount(stats.UploadRate)+"/s",
				"peers", stats.PeersUnique,
				"conns", stats.ConnectionsTotal,
				"files", stats.FilesTotal,
				"alloc", common.ByteCount(m.Alloc), "sys", common.ByteCount(m.Sys),
			)

			if stats.PeersUnique == 0 {
				ips := d.TorrentClient().BadPeerIPs()
				if len(ips) > 0 {
					d.logger.Info("[snapshots] Stats", "banned", ips)
				}
			}
		}
	}
}

func (d *Downloader) SnapDir() string { return d.cfg.Dirs.Snap }

func (d *Downloader) ReCalcStats(interval time.Duration) {
	//Call this methods outside of `statsLock` critical section, because they have own locks with contention
	torrents := d.torrentClient.Torrents()
	connStats := d.torrentClient.ConnStats()
	peers := make(map[torrent.PeerID]struct{}, 16)

	d.statsLock.Lock()
	defer d.statsLock.Unlock()
	prevStats, stats := d.stats, d.stats

	stats.Completed = true
	stats.BytesDownload = uint64(connStats.BytesReadUsefulIntendedData.Int64())
	stats.BytesUpload = uint64(connStats.BytesWrittenData.Int64())

	stats.BytesTotal, stats.BytesCompleted, stats.ConnectionsTotal, stats.MetadataReady = atomic.LoadUint64(&stats.DroppedTotal), atomic.LoadUint64(&stats.DroppedCompleted), 0, 0

	var zeroProgress []string
	var noMetadata []string

	for _, t := range torrents {
		select {
		case <-t.GotInfo():
			stats.MetadataReady++
			peersOfThisFile := t.PeerConns()
			weebseedPeersOfThisFile := t.WebseedPeerConns()
			for _, peer := range peersOfThisFile {
				stats.ConnectionsTotal++
				peers[peer.PeerID] = struct{}{}
			}
			stats.BytesCompleted += uint64(t.BytesCompleted())
			stats.BytesTotal += uint64(t.Length())

			progress := float32(float64(100) * (float64(t.BytesCompleted()) / float64(t.Length())))
			if progress == 0 {
				zeroProgress = append(zeroProgress, t.Name())
			}

			d.logger.Log(d.verbosity, "[snapshots] progress", "file", t.Name(), "progress", fmt.Sprintf("%.2f%%", progress), "peers", len(peersOfThisFile), "webseeds", len(weebseedPeersOfThisFile))
			isDiagEnabled := diagnostics.TypeOf(diagnostics.SegmentDownloadStatistics{}).Enabled()
			if d.verbosity >= log.LvlInfo || isDiagEnabled {
				webseedRates, websRates := getWebseedsRatesForlogs(weebseedPeersOfThisFile)
				rates, peersRates := getPeersRatesForlogs(peersOfThisFile)
				// more detailed statistic: download rate of each peer (for each file)
				if !t.Complete.Bool() && progress != 0 {
					d.logger.Info(fmt.Sprintf("[snapshots] webseed peers file=%s", t.Name()), webseedRates...)
					d.logger.Info(fmt.Sprintf("[snapshots] bittorrent peers file=%s", t.Name()), rates...)
				}

				if isDiagEnabled {
					diagnostics.Send(diagnostics.SegmentDownloadStatistics{
						Name:            t.Name(),
						TotalBytes:      uint64(t.Length()),
						DownloadedBytes: uint64(t.BytesCompleted()),
						WebseedsCount:   len(weebseedPeersOfThisFile),
						PeersCount:      len(peersOfThisFile),
						WebseedsRate:    websRates,
						PeersRate:       peersRates,
					})
				}
			}

		default:
			noMetadata = append(noMetadata, t.Name())
		}

		stats.Completed = stats.Completed && t.Complete.Bool()
	}

	if len(noMetadata) > 0 {
		amount := len(noMetadata)
		if len(noMetadata) > 5 {
			noMetadata = append(noMetadata[:5], "...")
		}
		d.logger.Log(d.verbosity, "[snapshots] no metadata yet", "files", amount, "list", strings.Join(noMetadata, ","))
	}
	if len(zeroProgress) > 0 {
		amount := len(zeroProgress)
		if len(zeroProgress) > 5 {
			zeroProgress = append(zeroProgress[:5], "...")
		}
		d.logger.Log(d.verbosity, "[snapshots] no progress yet", "files", amount, "list", strings.Join(zeroProgress, ","))
	}

	stats.DownloadRate = (stats.BytesDownload - prevStats.BytesDownload) / uint64(interval.Seconds())
	stats.UploadRate = (stats.BytesUpload - prevStats.BytesUpload) / uint64(interval.Seconds())

	if stats.BytesTotal == 0 {
		stats.Progress = 0
	} else {
		stats.Progress = float32(float64(100) * (float64(stats.BytesCompleted) / float64(stats.BytesTotal)))
		if int(stats.Progress) == 100 && !stats.Completed {
			stats.Progress = 99.99
		}
	}
	stats.PeersUnique = int32(len(peers))
	stats.FilesTotal = int32(len(torrents))

	d.stats = stats
}

func getWebseedsRatesForlogs(weebseedPeersOfThisFile []*torrent.Peer) ([]interface{}, uint64) {
	totalRate := uint64(0)
	averageRate := uint64(0)
	webseedRates := make([]interface{}, 0, len(weebseedPeersOfThisFile)*2)
	for _, peer := range weebseedPeersOfThisFile {
		urlS := strings.Trim(strings.TrimPrefix(peer.String(), "webseed peer for "), "\"")
		if urlObj, err := url.Parse(urlS); err == nil {
			if shortUrl, err := url.JoinPath(urlObj.Host, urlObj.Path); err == nil {
				rate := uint64(peer.DownloadRate())
				totalRate += rate
				webseedRates = append(webseedRates, shortUrl, fmt.Sprintf("%s/s", common.ByteCount(rate)))
			}
		}
	}

	lenght := uint64(len(weebseedPeersOfThisFile))
	if lenght > 0 {
		averageRate = totalRate / lenght
	}

	return webseedRates, averageRate
}

func getPeersRatesForlogs(peersOfThisFile []*torrent.PeerConn) ([]interface{}, uint64) {
	totalRate := uint64(0)
	averageRate := uint64(0)
	rates := make([]interface{}, 0, len(peersOfThisFile)*2)

	for _, peer := range peersOfThisFile {
		dr := uint64(peer.DownloadRate())
		rates = append(rates, peer.PeerClientName.Load(), fmt.Sprintf("%s/s", common.ByteCount(dr)))
		totalRate += dr
	}

	lenght := uint64(len(peersOfThisFile))
	if lenght > 0 {
		averageRate = totalRate / uint64(len(peersOfThisFile))
	}

	return rates, averageRate
}

func VerifyFile(ctx context.Context, t *torrent.Torrent, completePieces *atomic.Uint64) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.GotInfo():
	}

	g := &errgroup.Group{}
	for i := 0; i < t.NumPieces(); i++ {
		i := i
		g.Go(func() error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			t.Piece(i).VerifyData()
			completePieces.Add(1)
			return nil
		})
		//<-t.Complete.On()
	}
	return g.Wait()
}

func (d *Downloader) VerifyData(ctx context.Context, onlyFiles []string) error {
	total := 0
	_torrents := d.torrentClient.Torrents()
	torrents := make([]*torrent.Torrent, 0, len(_torrents))
	for _, t := range torrents {
		select {
		case <-t.GotInfo():
			if len(onlyFiles) > 0 && !slices.Contains(onlyFiles, t.Name()) {
				continue
			}
			torrents = append(torrents, t)
			total += t.NumPieces()
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	completedPieces := &atomic.Uint64{}

	{
		d.logger.Info("[snapshots] Verify start")
		defer d.logger.Info("[snapshots] Verify done")
		logEvery := time.NewTicker(20 * time.Second)
		defer logEvery.Stop()
		d.wg.Add(1)
		go func() {
			defer d.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-logEvery.C:
					d.logger.Info("[snapshots] Verify", "progress", fmt.Sprintf("%.2f%%", 100*float64(completedPieces.Load())/float64(total)))
				}
			}
		}()
	}

	g, ctx := errgroup.WithContext(ctx)
	// torrent lib internally limiting amount of hashers per file
	// set limit here just to make load predictable, not to control Disk/CPU consumption
	g.SetLimit(runtime.GOMAXPROCS(-1) * 4)

	for _, t := range torrents {
		t := t
		g.Go(func() error {
			return VerifyFile(ctx, t, completedPieces)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}
	// force fsync of db. to not loose results of validation on power-off
	return d.db.Update(context.Background(), func(tx kv.RwTx) error { return nil })
}

// AddNewSeedableFile decides what we do depending on wether we have the .seg file or the .torrent file
// have .torrent no .seg => get .seg file from .torrent
// have .seg no .torrent => get .torrent from .seg
func (d *Downloader) AddNewSeedableFile(ctx context.Context, name string) error {
	ff, ok := snaptype.ParseFileName("", name)
	if ok {
		if !ff.Seedable() {
			return nil
		}
	} else {
		if !e3seedable(name) {
			return nil
		}
	}

	// if we don't have the torrent file we build it if we have the .seg file
	torrentFilePath, err := BuildTorrentIfNeed(ctx, name, d.SnapDir())
	if err != nil {
		return fmt.Errorf("AddNewSeedableFile: %w", err)
	}
	ts, err := loadTorrent(torrentFilePath)
	if err != nil {
		return fmt.Errorf("AddNewSeedableFile: %w", err)
	}
	err = addTorrentFile(ctx, ts, d.torrentClient, d.webseeds)
	if err != nil {
		return fmt.Errorf("addTorrentFile: %w", err)
	}
	return nil
}

func (d *Downloader) alreadyHaveThisName(name string) bool {
	for _, t := range d.torrentClient.Torrents() {
		select {
		case <-t.GotInfo():
			if t.Name() == name {
				return true
			}
		default:
		}
	}
	return false
}

func (d *Downloader) AddMagnetLink(ctx context.Context, infoHash metainfo.Hash, name string) error {
	// Paranoic Mode on: if same file changed infoHash - skip it
	// Example:
	//  - Erigon generated file X with hash H1. User upgraded Erigon. New version has preverified file X with hash H2. Must ignore H2 (don't send to Downloader)
	if d.alreadyHaveThisName(name) {
		return nil
	}
	if d.newDownloadsAreProhibited() {
		return nil
	}

	mi := &metainfo.MetaInfo{AnnounceList: Trackers}
	magnet := mi.Magnet(&infoHash, &metainfo.Info{Name: name})
	spec, err := torrent.TorrentSpecFromMagnetUri(magnet.String())
	if err != nil {
		return err
	}
	spec.DisallowDataDownload = true
	t, _, err := d.torrentClient.AddTorrentSpec(spec)
	if err != nil {
		return err
	}
	d.wg.Add(1)
	go func(t *torrent.Torrent) {
		defer d.wg.Done()
		select {
		case <-ctx.Done():
			return
		case <-t.GotInfo():
		}

		mi := t.Metainfo()
		if err := CreateTorrentFileIfNotExists(d.SnapDir(), t.Info(), &mi); err != nil {
			d.logger.Warn("[snapshots] create torrent file", "err", err)
			return
		}
		urls, ok := d.webseeds.ByFileName(t.Name())
		if ok {
			t.AddWebSeeds(urls)
		}
	}(t)
	//log.Debug("[downloader] downloaded both seg and torrent files", "hash", infoHash)
	return nil
}

func seedableFiles(dirs datadir.Dirs) ([]string, error) {
	files, err := seedableSegmentFiles(dirs.Snap)
	if err != nil {
		return nil, fmt.Errorf("seedableSegmentFiles: %w", err)
	}
	l, err := seedableSnapshotsBySubDir(dirs.Snap, "history")
	if err != nil {
		return nil, err
	}
	l2, err := seedableSnapshotsBySubDir(dirs.Snap, "warm")
	if err != nil {
		return nil, err
	}
	files = append(append(files, l...), l2...)
	return files, nil
}
func (d *Downloader) addTorrentFilesFromDisk(quiet bool) error {
	logEvery := time.NewTicker(20 * time.Second)
	defer logEvery.Stop()

	files, err := AllTorrentSpecs(d.cfg.Dirs)
	if err != nil {
		return err
	}
	for i, ts := range files {
		err := addTorrentFile(d.ctx, ts, d.torrentClient, d.webseeds)
		if err != nil {
			return err
		}
		select {
		case <-logEvery.C:
			if !quiet {
				log.Info("[snapshots] Adding .torrent files", "progress", fmt.Sprintf("%d/%d", i, len(files)))
			}
		default:
		}
	}
	return nil
}
func (d *Downloader) BuildTorrentFilesIfNeed(ctx context.Context) error {
	return BuildTorrentFilesIfNeed(ctx, d.cfg.Dirs)
}
func (d *Downloader) Stats() AggStats {
	d.statsLock.RLock()
	defer d.statsLock.RUnlock()
	return d.stats
}

func (d *Downloader) Close() {
	d.stopMainLoop()
	d.wg.Wait()
	d.torrentClient.Close()
	if err := d.folder.Close(); err != nil {
		d.logger.Warn("[snapshots] folder.close", "err", err)
	}
	if err := d.pieceCompletionDB.Close(); err != nil {
		d.logger.Warn("[snapshots] pieceCompletionDB.close", "err", err)
	}
	d.db.Close()
}

func (d *Downloader) PeerID() []byte {
	peerID := d.torrentClient.PeerID()
	return peerID[:]
}

func (d *Downloader) StopSeeding(hash metainfo.Hash) error {
	t, ok := d.torrentClient.Torrent(hash)
	if !ok {
		return nil
	}
	ch := t.Closed()
	t.Drop()
	<-ch
	return nil
}

func (d *Downloader) TorrentClient() *torrent.Client { return d.torrentClient }

func openClient(ctx context.Context, dbDir, snapDir string, cfg *torrent.ClientConfig) (db kv.RwDB, c storage.PieceCompletion, m storage.ClientImplCloser, torrentClient *torrent.Client, err error) {
	db, err = mdbx.NewMDBX(log.New()).
		Label(kv.DownloaderDB).
		WithTableCfg(func(defaultBuckets kv.TableCfg) kv.TableCfg { return kv.DownloaderTablesCfg }).
		GrowthStep(16 * datasize.MB).
		MapSize(16 * datasize.GB).
		PageSize(uint64(8 * datasize.KB)).
		Path(dbDir).
		Open(ctx)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("torrentcfg.openClient: %w", err)
	}
	c, err = NewMdbxPieceCompletion(db)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("torrentcfg.NewMdbxPieceCompletion: %w", err)
	}
	m = storage.NewMMapWithCompletion(snapDir, c)
	cfg.DefaultStorage = m

	torrentClient, err = torrent.NewClient(cfg)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("torrent.NewClient: %w", err)
	}

	return db, c, m, torrentClient, nil
}

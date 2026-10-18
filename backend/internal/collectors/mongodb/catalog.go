// Package mongodb collects bounded catalog metadata without reading documents
// or executing write commands against the audited database.
package mongodb

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/osmendes/db-auditor/internal/config"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	MaxCollections = 1000
	MaxIndexes     = 20_000
)

type Index struct {
	Name       string
	Keys       []string
	Definition string
	Unique     bool
	Primary    bool
}

type Collection struct {
	Name         string
	Documents    int64
	DataBytes    int64
	StorageBytes int64
	IndexBytes   int64
	Indexes      []Index
}

type Catalog struct {
	Database    string
	Collections []Collection
}

// Inspect opens a direct, TLS-verified connection configured by the operator.
// The only server operations are ping, listCollections, collStats and listIndexes.
// Every returned error is redacted against uri, so the password never reaches
// the run record, the API or the logs.
func Inspect(ctx context.Context, uri, database string) (Catalog, error) {
	catalog, err := inspectCatalog(ctx, uri, database)
	if err != nil {
		return Catalog{}, config.RedactError(err, uri)
	}
	return catalog, nil
}

// inspectCatalog talks to the driver; tests replace it to inject driver errors.
var inspectCatalog = inspect

func inspect(ctx context.Context, uri, database string) (Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(uri).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(5 * time.Second).
		SetMaxPoolSize(2))
	if err != nil {
		return Catalog{}, fmt.Errorf("conectar ao MongoDB: %w", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = client.Disconnect(closeCtx)
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return Catalog{}, fmt.Errorf("verificar conexão MongoDB: %w", err)
	}
	db := client.Database(database)
	cursor, err := db.ListCollections(ctx, bson.D{})
	if err != nil {
		return Catalog{}, fmt.Errorf("listar coleções MongoDB: %w", err)
	}
	type collectionInfo struct {
		Name string `bson:"name"`
		Type string `bson:"type"`
	}
	infos := []collectionInfo{}
	for cursor.Next(ctx) {
		var info collectionInfo
		if err := cursor.Decode(&info); err != nil {
			_ = cursor.Close(ctx)
			return Catalog{}, err
		}
		if info.Type != "collection" || strings.HasPrefix(info.Name, "system.") {
			continue
		}
		infos = append(infos, info)
		if len(infos) > MaxCollections {
			_ = cursor.Close(ctx)
			return Catalog{}, fmt.Errorf("mais de %d coleções; divida o escopo antes de coletar", MaxCollections)
		}
	}
	err = cursor.Err()
	_ = cursor.Close(ctx)
	if err != nil {
		return Catalog{}, err
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name < infos[j].Name })
	result := Catalog{Database: database, Collections: make([]Collection, 0, len(infos))}
	totalIndexes := 0
	for _, info := range infos {
		commandCtx, stop := context.WithTimeout(ctx, 5*time.Second)
		var stats struct {
			Count          int64 `bson:"count"`
			Size           int64 `bson:"size"`
			StorageSize    int64 `bson:"storageSize"`
			TotalIndexSize int64 `bson:"totalIndexSize"`
		}
		err = db.RunCommand(commandCtx, bson.D{{Key: "collStats", Value: info.Name}}).Decode(&stats)
		stop()
		if err != nil {
			return Catalog{}, fmt.Errorf("estatísticas da coleção %q: %w", info.Name, err)
		}
		collection := Collection{Name: info.Name, Documents: stats.Count, DataBytes: stats.Size,
			StorageBytes: stats.StorageSize, IndexBytes: stats.TotalIndexSize}
		indexCursor, err := db.Collection(info.Name).Indexes().List(ctx)
		if err != nil {
			return Catalog{}, fmt.Errorf("índices da coleção %q: %w", info.Name, err)
		}
		for indexCursor.Next(ctx) {
			var item struct {
				Name   string `bson:"name"`
				Key    bson.D `bson:"key"`
				Unique bool   `bson:"unique"`
			}
			if err := indexCursor.Decode(&item); err != nil {
				_ = indexCursor.Close(ctx)
				return Catalog{}, err
			}
			totalIndexes++
			if totalIndexes > MaxIndexes {
				_ = indexCursor.Close(ctx)
				return Catalog{}, fmt.Errorf("mais de %d índices; divida o escopo antes de coletar", MaxIndexes)
			}
			keys, definitions := make([]string, 0, len(item.Key)), make([]string, 0, len(item.Key))
			for _, field := range item.Key {
				keys = append(keys, field.Key)
				definitions = append(definitions, field.Key+":"+fmt.Sprint(field.Value))
			}
			collection.Indexes = append(collection.Indexes, Index{Name: item.Name, Keys: keys,
				Definition: strings.Join(definitions, ", "), Unique: item.Unique || item.Name == "_id_", Primary: item.Name == "_id_"})
		}
		err = indexCursor.Err()
		_ = indexCursor.Close(ctx)
		if err != nil {
			return Catalog{}, err
		}
		result.Collections = append(result.Collections, collection)
	}
	return result, nil
}

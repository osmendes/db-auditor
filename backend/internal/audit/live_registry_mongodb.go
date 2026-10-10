package audit

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mayconmendes-qc/db-auditor/internal/collectors/mongodb"
	"github.com/mayconmendes-qc/db-auditor/internal/collectors/postgres"
)

// NewMongoRegistry maps collection and index metadata to the existing bounded
// inventory store. No PostgreSQL analyzer is run for this registry.
func NewMongoRegistry(targets map[string]string, writer InventoryWriter) *Registry {
	registry := NewRegistry()
	_ = registry.Register(CollectorSpec{
		Name: "mongodb.catalog", Version: "1.0.0", Profiles: DefaultProfiles(),
		ReadOnly: true, MaxRows: mongodb.MaxCollections + mongodb.MaxIndexes + 1,
		Run: func(ctx context.Context) (int64, error) {
			uri, err := dsnFromContext(ctx, targets)
			if err != nil {
				return 0, fmt.Errorf("URI MongoDB não configurada para o ambiente: %w", err)
			}
			u, err := url.Parse(uri)
			if err != nil {
				return 0, fmt.Errorf("URI MongoDB inválida")
			}
			database := strings.Trim(u.Path, "/")
			catalog, err := mongodb.Inspect(ctx, uri, database)
			if err != nil {
				return 0, err
			}
			envID, runID, err := runIDs(ctx)
			if err != nil {
				return 0, err
			}
			tables := make([]postgres.TableFacts, 0, len(catalog.Collections))
			indexes := make([]postgres.IndexFacts, 0)
			var totalBytes int64
			for _, collection := range catalog.Collections {
				size := collection.StorageBytes + collection.IndexBytes
				totalBytes += size
				tables = append(tables, postgres.TableFacts{
					DatabaseName: catalog.Database, SchemaName: "collections", TableName: collection.Name,
					Relkind: "r", RelationClass: "collection", DataSizeBytes: collection.DataBytes,
					IndexSizeBytes: collection.IndexBytes, TotalSizeBytes: size, RowEstimate: collection.Documents,
					StorageParameters: []string{},
					HasPrimaryKey:     true, // every MongoDB collection has a unique _id index
				})
				for _, index := range collection.Indexes {
					indexes = append(indexes, postgres.IndexFacts{
						DatabaseName: catalog.Database, SchemaName: "collections", TableName: collection.Name,
						IndexName: collection.Name + "/" + index.Name, IndexDefinition: index.Definition,
						AccessMethod: "mongodb", IsUnique: index.Unique, IsPrimary: index.Primary,
						IsValid: true, IsReady: true, KeyColumns: index.Keys,
					})
				}
			}
			if err := writer.SaveDiscovery(ctx, envID, runID,
				[]postgres.DatabaseFacts{{Name: catalog.Database, AllowConnections: true, SizeBytes: totalBytes}},
				[]postgres.SchemaFacts{{DatabaseName: catalog.Database, SchemaName: "collections", TableCount: len(tables), SizeBytes: totalBytes}}); err != nil {
				return 0, err
			}
			if err := writer.SaveObjectInventory(ctx, envID, runID, tables, nil, indexes); err != nil {
				return 0, err
			}
			return int64(1 + len(tables) + len(indexes)), nil
		},
	})
	return registry
}

package iavl

import (
	"github.com/pkg/errors"

	dbm "github.com/tendermint/tmlibs/db"
)

type IAVLVersionedTree struct {
	// The current (latest) version of the tree.
	*IAVLOrphaningTree

	versions map[uint64]*IAVLOrphaningTree
	ndb      *nodeDB
}

func NewIAVLVersionedTree(cacheSize int, db dbm.DB) *IAVLVersionedTree {
	ndb := newNodeDB(cacheSize, db)
	head := &IAVLTree{ndb: ndb}

	return &IAVLVersionedTree{
		IAVLOrphaningTree: NewIAVLPersistentTree(head),
		versions:          map[uint64]*IAVLOrphaningTree{},
		ndb:               ndb,
	}
}

func (tree *IAVLVersionedTree) String() string {
	return tree.ndb.String()
}

func (tree *IAVLVersionedTree) Load() error {
	roots, err := tree.ndb.getRoots()
	if err != nil {
		return err
	}

	var latest uint64
	for _, root := range roots {
		t := NewIAVLPersistentTree(&IAVLTree{ndb: tree.ndb})
		t.Load(root)

		version := t.root.version
		tree.versions[version] = t

		if version > latest {
			latest = version
		}
	}
	tree.IAVLTree = tree.versions[latest].Copy()

	return nil
}

func (tree *IAVLVersionedTree) GetVersioned(key []byte, version uint64) (
	index int, value []byte, exists bool,
) {
	if t, ok := tree.versions[version]; ok {
		return t.Get(key)
	}
	return -1, nil, false
}

func (tree *IAVLVersionedTree) DeleteVersion(version uint64) error {
	if t, ok := tree.versions[version]; ok {
		// TODO: Use version parameter.
		tree.ndb.DeleteOrphans(t.root.version)
		tree.ndb.DeleteRoot(t.root.version)
		tree.ndb.Commit()

		// TODO: Not necessary.
		t.root.leftNode = nil
		t.root.rightNode = nil

		delete(tree.versions, version)

		return nil
	}
	// TODO: What happens if you delete HEAD?
	return errors.Errorf("version %d does not exist", version)
}

func (tree *IAVLVersionedTree) SaveVersion(version uint64) error {
	if _, ok := tree.versions[version]; ok {
		return errors.Errorf("version %d was already saved", version)
	}
	if tree.root == nil {
		return errors.New("tree is empty")
	}
	if version == 0 {
		return errors.New("version must be greater than zero")
	}
	tree.versions[version] = tree.IAVLOrphaningTree

	tree.ndb.SaveBranch(tree.root, version, func(hash []byte) {
		tree.deleteOrphan(hash)

		for _, t := range tree.versions {
			if version, ok := t.deleteOrphan(hash); ok {
				tree.ndb.Unorphan(hash, version)
			}
		}
	})
	tree.ndb.SaveRoot(tree.root)
	tree.ndb.SaveOrphans(tree.orphans)
	tree.ndb.Commit()
	tree.IAVLOrphaningTree = NewIAVLPersistentTree(tree.Copy())

	return nil
}
